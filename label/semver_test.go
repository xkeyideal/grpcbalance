package label

import (
	"strconv"
	"testing"
)

func TestSatisfy(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		want    bool
		wantErr bool
	}{
		{"^0.2.3", "0.2.9", true, false},
		{"^0.2.3", "0.3.0", false, false},
		{"^0.2.3", "0.2.1", false, false},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			got, err := Satisfy(tt.constraint, tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("Satisfy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Satisfy() got = %v, want %v", got, tt.want)
			}
		})
	}
}
