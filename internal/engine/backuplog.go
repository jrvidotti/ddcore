package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jrvidotti/ddcore/internal/db"
)

// BackupStatus is the newest successful `ddcore backup` this database knows
// of. It knows only of archives the CLI wrote: a platform snapshot leaves no
// row, which is why the age check is off unless a site opts in.
type BackupStatus struct {
	Finished time.Time `json:"finished"`
	Location string    `json:"location"`
	Bytes    int64     `json:"bytes"`
	// Failures counts failed backups since that one.
	Failures int `json:"failures"`
}

// LastBackup returns nil when no backup ever succeeded here.
func (e *Engine) LastBackup(ctx context.Context) (*BackupStatus, error) {
	var b BackupStatus
	err := e.DB.Pool.QueryRow(ctx, `SELECT finished, location, bytes FROM ddcore_backup_log
		WHERE kind = 'backup' AND ok ORDER BY finished DESC LIMIT 1`).Scan(&b.Finished, &b.Location, &b.Bytes)
	if errors.Is(err, pgx.ErrNoRows) || db.UndefinedTable(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM ddcore_backup_log
		WHERE kind = 'backup' AND NOT ok AND finished > $1`, b.Finished).Scan(&b.Failures); err != nil {
		return nil, err
	}
	return &b, nil
}

// backupWarning is the health report's sentence about backup age, or "".
func (e *Engine) backupWarning(ctx context.Context) string {
	limit := e.Cfg.Ops.BackupMaxAgeHours
	if limit <= 0 || e.DB == nil {
		return ""
	}
	b, err := e.LastBackup(ctx)
	if err != nil {
		return "backups could not be read: " + db.RedactError(err)
	}
	if b == nil {
		return fmt.Sprintf("no successful backup recorded (ops.backupMaxAgeHours is %d)", limit)
	}
	if age := time.Since(b.Finished); age > time.Duration(limit)*time.Hour {
		return fmt.Sprintf("newest backup is %s old (limit %dh)", age.Round(time.Minute), limit)
	}
	return ""
}
