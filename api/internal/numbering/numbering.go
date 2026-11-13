// Package numbering hands out gap-free sequential numbers inside the caller's
// transaction: the row lock on the series is held until commit, so two
// concurrent transactions never get the same number and a rolled-back
// transaction never leaves a gap.
package numbering

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Pupil returns the next pupil number for a year: NV-2027-0001.
func Pupil(ctx context.Context, tx pgx.Tx, year int) (string, error) {
	var n int
	err := tx.QueryRow(ctx, `
		INSERT INTO pupil_number_series (year, next_value) VALUES ($1, 2)
		ON CONFLICT (year) DO UPDATE SET next_value = pupil_number_series.next_value + 1
		RETURNING next_value - 1`, year).Scan(&n)
	if err != nil {
		return "", fmt.Errorf("pupil number: %w", err)
	}
	return fmt.Sprintf("NV-%d-%04d", year, n), nil
}

// ErrNoSeries means the series was not configured for that entity and year.
var ErrNoSeries = errors.New("numbering series not configured")

// Document returns the next number of an entity's series (receipt,
// statement) for a year: REC-2027-00001.
func Document(ctx context.Context, tx pgx.Tx, entityID uuid.UUID, series string, year int) (string, error) {
	var prefix string
	var n int
	err := tx.QueryRow(ctx, `
		UPDATE number_series SET next_value = next_value + 1
		WHERE entity_id = $1 AND series = $2 AND year = $3
		RETURNING prefix, next_value - 1`, entityID, series, year).Scan(&prefix, &n)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoSeries
	}
	if err != nil {
		return "", fmt.Errorf("document number: %w", err)
	}
	return fmt.Sprintf("%s-%d-%05d", prefix, year, n), nil
}
