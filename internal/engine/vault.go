package engine

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// MasterKeyEnvName is the environment variable that holds the master encryption key.
const MasterKeyEnvName = "DDCORE_SECRET_KEY"

// MasterKey derives a 32-byte key from DDCORE_SECRET_KEY using SHA-256.
func MasterKey() ([]byte, error) {
	val := strings.TrimSpace(os.Getenv(MasterKeyEnvName))
	if val == "" {
		return nil, cerr.Validation("Vault secret key ({0}) is not configured on this site", MasterKeyEnvName)
	}
	hash := sha256.Sum256([]byte(val))
	return hash[:], nil
}

// EncryptVault encrypts a plaintext string using AES-256-GCM.
func EncryptVault(key []byte, plaintext string) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return ciphertext, nonce, nil
}

// DecryptVault decrypts ciphertext using AES-256-GCM.
func DecryptVault(key, ciphertext, nonce []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", cerr.Validation("Failed to decrypt vault secret: authentication error")
	}
	return string(plaintext), nil
}

func (e *Engine) recordVaultAudit(c *Ctx, secretName, action string) {
	auditAction := "vault." + action
	if c != nil && c.E != nil {
		_ = c.Audit(auditAction, "Vault Secret", secretName, nil)
		return
	}
	ctx := context.Background()
	_, _ = e.DB.Pool.Exec(ctx, `INSERT INTO tab_audit_event
		(id, owner, creation, modified, modified_by, docstatus, action, outcome, actor, target_doctype, target_id, ip, request_id, detail)
		VALUES ($1, 'System', now(), now(), 'System', 0, $2, 'Allowed', 'System', 'Vault Secret', $3, NULL, NULL, NULL)`,
		RandomToken(), auditAction, secretName)
}

// VaultSet writes an encrypted secret to ddcore_vault and records an audit log entry.
func (e *Engine) VaultSet(c *Ctx, name, value string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return cerr.Validation("Vault secret name cannot be empty")
	}
	key, err := MasterKey()
	if err != nil {
		return err
	}
	ciphertext, nonce, err := EncryptVault(key, value)
	if err != nil {
		return err
	}

	var q db.Querier
	var ctx context.Context
	if c != nil {
		q = c.Q()
		ctx = c.Ctx
	} else {
		q = e.DB.Pool
		ctx = context.Background()
	}

	query := `
INSERT INTO ddcore_vault (name, ciphertext, nonce, updated)
VALUES ($1, $2, $3, now())
ON CONFLICT (name) DO UPDATE
SET ciphertext = EXCLUDED.ciphertext, nonce = EXCLUDED.nonce, updated = now();`

	if _, err := q.Exec(ctx, query, name, ciphertext, nonce); err != nil {
		return err
	}

	e.recordVaultAudit(c, name, "write")
	return nil
}

// VaultGet retrieves and decrypts a secret from ddcore_vault, recording an audit log entry.
func (e *Engine) VaultGet(c *Ctx, name string) (string, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false, nil
	}
	key, err := MasterKey()
	if err != nil {
		return "", false, err
	}

	var q db.Querier
	var ctx context.Context
	if c != nil {
		q = c.Q()
		ctx = c.Ctx
	} else {
		q = e.DB.Pool
		ctx = context.Background()
	}

	plaintext, ok, err := vaultRead(ctx, q, key, name)
	if err != nil || !ok {
		return "", ok, err
	}
	e.recordVaultAudit(c, name, "read")
	return plaintext, true, nil
}

// vaultRead decrypts one secret and records nothing.
//
// Only for the framework reading a key it owns on its own schedule — a webhook
// signing each attempt. An audit row per retry would bury the reads a person
// made under the ones a worker made, which is the opposite of what the log is
// for. Anything an app or a person asks for goes through VaultGet.
func vaultRead(ctx context.Context, q db.Querier, key []byte, name string) (string, bool, error) {
	var ciphertext, nonce []byte
	err := q.QueryRow(ctx, `SELECT ciphertext, nonce FROM ddcore_vault WHERE name = $1`, name).Scan(&ciphertext, &nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	plaintext, err := DecryptVault(key, ciphertext, nonce)
	if err != nil {
		return "", false, err
	}
	return plaintext, true, nil
}

// VaultDel removes a secret from ddcore_vault and records an audit log entry.
func (e *Engine) VaultDel(c *Ctx, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}

	var q db.Querier
	var ctx context.Context
	if c != nil {
		q = c.Q()
		ctx = c.Ctx
	} else {
		q = e.DB.Pool
		ctx = context.Background()
	}

	if _, err := q.Exec(ctx, `DELETE FROM ddcore_vault WHERE name = $1`, name); err != nil {
		return err
	}

	e.recordVaultAudit(c, name, "delete")
	return nil
}

// VaultList returns all secret names starting with prefix, in alphabetical order.
// Values are never returned.
func (e *Engine) VaultList(c *Ctx, prefix string) ([]string, error) {
	var q db.Querier
	var ctx context.Context
	if c != nil {
		q = c.Q()
		ctx = c.Ctx
	} else {
		q = e.DB.Pool
		ctx = context.Background()
	}

	rows, err := q.Query(ctx, `SELECT name FROM ddcore_vault WHERE name LIKE $1 || '%' ORDER BY name ASC`, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	if names == nil {
		names = []string{}
	}
	return names, rows.Err()
}

// VaultHas checks whether a secret exists in ddcore_vault without decrypting or auditing read.
func (e *Engine) VaultHas(ctx context.Context, q db.Querier, name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, nil
	}
	var dummy int
	err := q.QueryRow(ctx, `SELECT 1 FROM ddcore_vault WHERE name = $1`, name).Scan(&dummy)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// VaultStatus reports whether the master key is configured, how many entries exist, and their names.
func (e *Engine) VaultStatus(ctx context.Context) (configured bool, count int, names []string, err error) {
	_, keyErr := MasterKey()
	configured = (keyErr == nil)

	rows, err := e.DB.Pool.Query(ctx, `SELECT name FROM ddcore_vault ORDER BY name ASC`)
	if err != nil {
		return configured, 0, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return configured, 0, nil, err
		}
		names = append(names, n)
	}
	return configured, len(names), names, rows.Err()
}

// DeriveVaultKey computes the vault key for a document field.
// If options template is provided (e.g. "asaas:token:{id}"), it interpolates document fields.
// Otherwise, it defaults to "<DocType>:<Fieldname>:<docName>".
func (c *Ctx) DeriveVaultKey(d *meta.DocType, f *meta.Field, doc Doc) string {
	tmpl := f.OptionsString()
	if tmpl == "" {
		return d.Name + ":" + f.Fieldname + ":" + doc.ID()
	}
	res := tmpl
	for k, v := range doc {
		token := "{" + k + "}"
		if strings.Contains(res, token) {
			res = strings.ReplaceAll(res, token, db.Str(v))
		}
	}
	if strings.Contains(res, "{id}") {
		res = strings.ReplaceAll(res, "{id}", doc.ID())
	}
	return res
}

func (c *Ctx) processVaultFields(d *meta.DocType, doc Doc) error {
	if d == nil || doc == nil {
		return nil
	}
	for _, f := range d.Fields {
		if f.Fieldtype == "Vault" {
			if err := c.processSingleVaultField(d, f, doc); err != nil {
				return err
			}
		} else if meta.IsTableType(f.Fieldtype) && f.OptionsString() != "" {
			child, err := c.St.DocType(f.OptionsString())
			if err != nil || child == nil {
				continue
			}
			for _, row := range doc.Children(f.Fieldname) {
				for _, cf := range child.Fields {
					if cf.Fieldtype == "Vault" {
						if err := c.processSingleVaultField(child, cf, row); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	delete(doc, "__vault_clear")
	return nil
}

func (c *Ctx) processSingleVaultField(d *meta.DocType, f *meta.Field, doc Doc) error {
	raw, exists := doc[f.Fieldname]
	key := c.DeriveVaultKey(d, f, doc)

	// Check if explicitly marked for clearing via doc["__vault_clear"]
	if clears, ok := doc["__vault_clear"].([]string); ok {
		for _, cn := range clears {
			if cn == f.Fieldname {
				return c.E.VaultDel(c, key)
			}
		}
	} else if clears, ok := doc["__vault_clear"].([]any); ok {
		for _, cn := range clears {
			if s, ok := cn.(string); ok && s == f.Fieldname {
				return c.E.VaultDel(c, key)
			}
		}
	}

	if !exists || raw == nil {
		return nil
	}

	switch v := raw.(type) {
	case string:
		if v != "" {
			return c.E.VaultSet(c, key, v)
		}
	case map[string]any:
		if clear, ok := v["clear"].(bool); ok && clear {
			return c.E.VaultDel(c, key)
		}
		if s, ok := v["value"].(string); ok && s != "" {
			return c.E.VaultSet(c, key, s)
		}
		if s, ok := v["secret"].(string); ok && s != "" {
			return c.E.VaultSet(c, key, s)
		}
	}
	return nil
}
