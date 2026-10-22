package engine

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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

// PreviousKeyEnvName holds the master key a rotation is moving away from. The
// server still reads with it, never writes with it, and `ddcore vault rekey`
// moves every secret still under it to the current key (#134).
const PreviousKeyEnvName = "DDCORE_SECRET_KEY_PREVIOUS"

// MasterKey derives a 32-byte key from DDCORE_SECRET_KEY using SHA-256.
func MasterKey() ([]byte, error) {
	kr, err := VaultKeys()
	if err != nil {
		return nil, err
	}
	return kr.current.key, nil
}

// vaultKey is a derived master key and its fingerprint.
type vaultKey struct {
	key []byte
	// id is what a ddcore_vault row records of the key that encrypted it:
	// enough to tell two keys apart, nothing to recover either from.
	id string
}

func deriveVaultKey(val string) vaultKey {
	hash := sha256.Sum256([]byte(val))
	mac := hmac.New(sha256.New, hash[:])
	mac.Write([]byte("ddcore-vault-key-id"))
	return vaultKey{key: hash[:], id: hex.EncodeToString(mac.Sum(nil)[:8])}
}

// vaultKeyring is the key every write uses and, during a rotation, the one
// the secrets not yet re-keyed were written with.
type vaultKeyring struct {
	current  vaultKey
	previous *vaultKey
}

// VaultKeys reads the master key and, when one is set, the previous one. A
// previous key equal to the current one is no previous key at all.
func VaultKeys() (vaultKeyring, error) {
	val := strings.TrimSpace(os.Getenv(MasterKeyEnvName))
	if val == "" {
		return vaultKeyring{}, cerr.Validation("Vault secret key ({0}) is not configured on this site", MasterKeyEnvName)
	}
	kr := vaultKeyring{current: deriveVaultKey(val)}
	if prev := strings.TrimSpace(os.Getenv(PreviousKeyEnvName)); prev != "" && prev != val {
		p := deriveVaultKey(prev)
		kr.previous = &p
	}
	return kr, nil
}

// open decrypts a row. The key its fingerprint names goes first — the current
// one for a row written before fingerprints, or under a key this process does
// not know — and the other is tried when that one fails to authenticate, so a
// fingerprint left stale (an older binary rewriting the row) still opens. used
// is the key that opened it.
func (kr vaultKeyring) open(keyID string, ciphertext, nonce []byte) (plaintext string, used *vaultKey, err error) {
	order := []*vaultKey{&kr.current}
	if kr.previous != nil {
		if keyID == kr.previous.id {
			order = []*vaultKey{kr.previous, &kr.current}
		} else {
			order = append(order, kr.previous)
		}
	}
	for _, k := range order {
		if plaintext, err = DecryptVault(k.key, ciphertext, nonce); err == nil {
			return plaintext, k, nil
		}
	}
	return "", nil, err
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

func (e *Engine) recordVaultAudit(c *Ctx, secretName, action string, detail map[string]any) {
	auditAction := "vault." + action
	if c != nil && c.E != nil {
		_ = c.Audit(auditAction, "Vault Secret", secretName, detail)
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
	kr, err := VaultKeys()
	if err != nil {
		return err
	}
	ciphertext, nonce, err := EncryptVault(kr.current.key, value)
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

	// a tenant's secrets are its own, and so is the name each one goes by
	conflict := "name"
	if on, err := e.tenancy(ctx); err != nil {
		return err
	} else if on {
		conflict = "tenant, name"
	}
	query := `
INSERT INTO ddcore_vault (name, ciphertext, nonce, key_id, updated)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (` + conflict + `) DO UPDATE
SET ciphertext = EXCLUDED.ciphertext, nonce = EXCLUDED.nonce, key_id = EXCLUDED.key_id, updated = now();`

	if _, err := q.Exec(ctx, query, name, ciphertext, nonce, kr.current.id); err != nil {
		return vaultSchemaErr(err)
	}

	e.recordVaultAudit(c, name, "write", nil)
	return nil
}

// VaultGet retrieves and decrypts a secret from ddcore_vault, recording an audit log entry.
func (e *Engine) VaultGet(c *Ctx, name string) (string, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false, nil
	}
	kr, err := VaultKeys()
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

	plaintext, ok, err := vaultRead(ctx, q, kr, name)
	if err != nil || !ok {
		return "", ok, err
	}
	e.recordVaultAudit(c, name, "read", nil)
	return plaintext, true, nil
}

// platformOnly is what a statement on ddcore_vault adds to its WHERE to mean
// the platform space's row and no other.
const platformOnly = " AND tenant = ''"

// platformVault runs fn on a querier that reaches the platform space's
// secrets from whatever space the ctx works in: the secret of a shared
// DocType's Vault field lives there, like the document it belongs to, and a
// tenant reads it through here. where is what fn's statement appends to name
// the platform's row; without tenancy there is no such column and it is empty.
func (c *Ctx) platformVault(fn func(q db.Querier, where string) error) error {
	if !c.Tenancy() {
		return fn(c.Q(), "")
	}
	if c.Tenant == "" {
		return fn(c.Q(), platformOnly)
	}
	if c.Tx == nil {
		return c.E.InSpace(c.Ctx, "", func(q db.Querier) error { return fn(q, platformOnly) })
	}
	return c.elevated(func() error { return fn(c.Tx, platformOnly) })
}

// VaultGetShared is VaultGet for a secret of the platform space, read from any
// space. The audit row lands in the space that read it and says the secret
// was the shared one.
func (e *Engine) VaultGetShared(c *Ctx, name string) (string, bool, error) {
	if c == nil {
		return e.VaultGet(nil, name)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false, nil
	}
	kr, err := VaultKeys()
	if err != nil {
		return "", false, err
	}
	var plaintext string
	var ok bool
	err = c.platformVault(func(q db.Querier, where string) error {
		var err error
		plaintext, ok, err = vaultReadWhere(c.Ctx, q, kr, name, where)
		return err
	})
	if err != nil || !ok {
		return "", false, err
	}
	e.recordVaultAudit(c, name, "read", map[string]any{"shared": true})
	return plaintext, true, nil
}

// sharedVaultWrite refuses a write that asks for the shared secret from
// inside a tenant: like a shared document, it is the platform's to change.
// In the platform space the shared secret is the space's own, and the write
// goes on as any other.
func (c *Ctx) sharedVaultWrite(opts map[string]any) error {
	if opts["shared"] == true && c.Tenant != "" && c.Tenancy() {
		return cerr.Permission("A shared vault secret can only be changed from the platform space")
	}
	return nil
}

// sharedVaultKeys are the names of the secrets the Vault fields of DocTypes
// with no tenant hold: the ones that stay in the platform space whatever
// else leaves it.
func (c *Ctx) sharedVaultKeys() ([]string, error) {
	keys := []string{}
	for _, name := range c.St.Meta.Names() {
		d := c.St.Meta.DocTypes[name]
		if d.TenantOwned || d.IsVirtual() {
			continue
		}
		var rows []map[string]any
		for _, f := range d.Fields {
			if f.Fieldtype != "Vault" {
				continue
			}
			if rows == nil {
				var err error
				rows, err = db.Select(c.Ctx, c.Q(), fmt.Sprintf("SELECT * FROM %s", db.Ident(d.TableName())))
				if err != nil {
					return nil, err
				}
			}
			for _, row := range rows {
				keys = append(keys, c.DeriveVaultKey(d, f, Doc(row)))
			}
		}
	}
	return keys, nil
}

// vaultFieldHas reports whether a document's Vault field has a secret. A
// DocType with no tenant keeps its secrets in the platform space, where every
// space finds them.
func (c *Ctx) vaultFieldHas(d *meta.DocType, key string) (bool, error) {
	if d.TenantOwned || d.IsVirtual() {
		return c.E.VaultHas(c.Ctx, c.Q(), key)
	}
	var has bool
	err := c.platformVault(func(q db.Querier, where string) error {
		var err error
		has, err = vaultHasWhere(c.Ctx, q, key, where)
		return err
	})
	return has, err
}

// errVaultNotMigrated is what the vault answers on a database the running
// binary has not migrated yet: a column it reads and writes is not there (#135).
func errVaultNotMigrated() error {
	return cerr.Validation("The vault has not been migrated to this version: run ddcore migrate")
}

// vaultSchemaErr names the way out when a vault query met a column that is not
// there, instead of passing Postgres's own words on.
func vaultSchemaErr(err error) error {
	if db.UndefinedColumn(err) {
		return errVaultNotMigrated()
	}
	return err
}

// vaultMigrated is checked before a rotation reads anything: a re-key that
// cannot start must not record that it ran.
func vaultMigrated(ctx context.Context, q db.Querier) error {
	var ok bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'ddcore_vault' AND column_name = 'key_id')`).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return errVaultNotMigrated()
	}
	return nil
}

// vaultRead decrypts one secret and records nothing.
//
// Only for the framework reading a key it owns on its own schedule — a webhook
// signing each attempt. An audit row per retry would bury the reads a person
// made under the ones a worker made, which is the opposite of what the log is
// for. Anything an app or a person asks for goes through VaultGet.
func vaultRead(ctx context.Context, q db.Querier, kr vaultKeyring, name string) (string, bool, error) {
	return vaultReadWhere(ctx, q, kr, name, "")
}

func vaultReadWhere(ctx context.Context, q db.Querier, kr vaultKeyring, name, where string) (string, bool, error) {
	var ciphertext, nonce []byte
	var keyID string
	err := q.QueryRow(ctx, `SELECT ciphertext, nonce, coalesce(key_id, '') FROM ddcore_vault WHERE name = $1`+where, name).Scan(&ciphertext, &nonce, &keyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, vaultSchemaErr(err)
	}
	plaintext, _, err := kr.open(keyID, ciphertext, nonce)
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

	e.recordVaultAudit(c, name, "delete", nil)
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
	return vaultHasWhere(ctx, q, name, "")
}

func vaultHasWhere(ctx context.Context, q db.Querier, name, where string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, nil
	}
	var dummy int
	err := q.QueryRow(ctx, `SELECT 1 FROM ddcore_vault WHERE name = $1`+where, name).Scan(&dummy)
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
