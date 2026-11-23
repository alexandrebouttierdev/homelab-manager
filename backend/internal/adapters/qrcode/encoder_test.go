package qrcode

import (
	"strings"
	"testing"
)

func TestEncodeReturnsTheModuleMatrix(t *testing.T) {
	matrix, err := New().Encode("otpauth://totp/Homelab%20Manager:alice?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ&issuer=Homelab+Manager")
	if err != nil {
		t.Fatal(err)
	}
	if matrix.Size < 21 || len(matrix.Rows) != matrix.Size {
		t.Fatalf("size %d with %d rows", matrix.Size, len(matrix.Rows))
	}
	for _, row := range matrix.Rows {
		if len(row) != matrix.Size || strings.Trim(row, "01") != "" {
			t.Fatalf("row %q is not %d modules of 0 and 1", row, matrix.Size)
		}
	}
	// Finder pattern in the top-left corner: a 7-module dark border.
	if !strings.HasPrefix(matrix.Rows[0], "1111111") || !strings.HasPrefix(matrix.Rows[6], "1111111") || !strings.HasPrefix(matrix.Rows[1], "1000001") {
		t.Fatalf("no finder pattern: %q %q", matrix.Rows[0][:7], matrix.Rows[1][:7])
	}
}
