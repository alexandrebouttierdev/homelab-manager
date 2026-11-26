// Package qrcode draws QR codes with rsc.io/qr. The matrix is returned to
// the interface, which renders it as SVG: no image is generated here.
package qrcode

import (
	"fmt"
	"strings"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"

	"rsc.io/qr"
)

var _ ports.QREncoder = (*Encoder)(nil)

type Encoder struct{}

func New() *Encoder { return &Encoder{} }

// Encode uses the medium error correction level (15 %), enough for a
// code read from a screen.
func (e *Encoder) Encode(text string) (domain.QRMatrix, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return domain.QRMatrix{}, fmt.Errorf("encode QR code: %w", err)
	}
	rows := make([]string, code.Size)
	for y := range code.Size {
		var row strings.Builder
		for x := range code.Size {
			if code.Black(x, y) {
				row.WriteByte('1')
			} else {
				row.WriteByte('0')
			}
		}
		rows[y] = row.String()
	}
	return domain.QRMatrix{Size: code.Size, Rows: rows}, nil
}
