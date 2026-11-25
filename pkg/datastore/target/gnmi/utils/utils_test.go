package utils

import "testing"

func TestParseEncodingConfig(t *testing.T) {
	if got := ParseGnmiEncoding("config"); got != 45 {
		t.Errorf("ParseGnmiEncoding(config) = %d, want 45", got)
	}
	if got := ParseSdcpbEncoding("CONFIG"); got != 45 {
		t.Errorf("ParseSdcpbEncoding(CONFIG) = %d, want 45", got)
	}
	if got := ParseGnmiEncoding("45"); got != 45 {
		t.Errorf("ParseGnmiEncoding(45) = %d, want 45", got)
	}
}
