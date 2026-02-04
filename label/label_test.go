package label

import (
	"reflect"
	"testing"
)

func TestParseLabels(t *testing.T) {
	type args struct {
		labels []string
	}
	tests := []struct {
		name    string
		args    args
		want    Labels
		wantErr bool
	}{
		{
			name: "1",
			args: args{
				labels: []string{"a=1", "a=2"},
			},
			want: labels{"a": Set{"1": Noop, "2": Noop}},
		},
		{
			name: "2",
			args: args{
				labels: []string{"a=1", "b=2"},
			},
			want: labels{"a": Set{"1": Noop}, "b": Set{"2": Noop}},
		},
		{
			name: "3",
			args: args{
				labels: []string{"a=2021-10-18T10:20:41+08:00"},
			},
			want: labels{"a": Set{"2021-10-18T10:20:41+08:00": Noop}},
		},
		{
			name: "CJK",
			args: args{
				labels: []string{"公布=PGS数据系统"},
			},
			want: labels{"公布": Set{"PGS数据系统": Noop}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLabels(tt.args.labels)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLabels() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseLabels() got = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLabelsEqual(t *testing.T) {
	a := labels{"a": Set{"1": Noop, "2": Noop}, "b": Set{"x": Noop}}
	b := labels{"b": Set{"x": Noop}, "a": Set{"2": Noop, "1": Noop}}

	if !Equal(a, b) {
		t.Fatalf("Equal(a,b)=false, want true")
	}
	if !a.Equal(b) {
		t.Fatalf("a.Equal(b)=false, want true")
	}

	c := labels{"a": Set{"1": Noop}, "b": Set{"x": Noop}}
	if Equal(a, c) {
		t.Fatalf("Equal(a,c)=true, want false")
	}
}
