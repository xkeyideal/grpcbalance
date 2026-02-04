package label

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	type args struct {
		selector string
	}
	tests := []struct {
		name    string
		args    args
		want    Selector
		wantErr bool
	}{
		{
			name: "1",
			args: args{
				selector: "system.node.kernel~=4.4.10",
			},
			want: internalSelector{
				Requirement{
					key:       "system.node.kernel",
					operator:  "~=",
					strValues: []string{"4.4.10"},
				},
			},
		}, {
			name: "2",
			args: args{
				selector: "system.node.kernel==4.4.10",
			},
			want: internalSelector{
				Requirement{
					key:       "system.node.kernel",
					operator:  DoubleEquals,
					strValues: []string{"4.4.10"},
				},
			},
		}, {
			name: "3",
			args: args{
				selector: "system.node.platform~=win*",
			},
			want: internalSelector{
				Requirement{
					key:       "system.node.platform",
					operator:  Pattern,
					strValues: []string{"win*"},
				},
			},
		}, {
			name: "4",
			args: args{
				selector: "app.version@<1.2.0 || >2",
			},
			want: internalSelector{
				Requirement{
					key:       "app.version",
					operator:  Semver,
					strValues: []string{"<1.2.0 || >2"},
				},
			},
		}, {
			name: "5",
			args: args{
				selector: "app.version@<1.2.0 || >2, app.name~=app-0?-*",
			},
			want: internalSelector{
				Requirement{
					key:       "app.name",
					operator:  Pattern,
					strValues: []string{"app-0?-*"},
				}, {
					key:       "app.version",
					operator:  Semver,
					strValues: []string{"<1.2.0 || >2"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSelector(tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSelector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseSelector() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSelector(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		want    Selector
		wantErr bool
	}{
		{"semver", "app.version @^1.0.1", internalSelector{Requirement{
			key:       "app.version",
			operator:  Semver,
			strValues: []string{"^1.0.1"},
		}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSelector(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSelector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseSelector() got = %v, want %v", got, tt.want)
			}
		})
	}
}
