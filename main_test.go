package main

import (
	"testing"
)

func Test_validAddr(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"empty", args{""}, true},
		{"only_port", args{":8080"}, false},
		{"only_addr", args{"localhost"}, true},
		{"full", args{"192.168.10.1:8181"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validAddr(tt.args.s); (err != nil) != tt.wantErr {
				t.Errorf("validAddr() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_trimFirst(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"empty", args{""}, ""},
		{"one", args{"a"}, ""},
		{"two", args{"ab"}, "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimFirst(tt.args.s); got != tt.want {
				t.Errorf("trimFirst() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_splitToKeyValue(t *testing.T) {
	type args struct {
		s   string
		sep string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		want1   string
		wantErr bool
	}{
		{"empty", args{"", ":"}, "", "", true},
		{"only_key", args{"key", ":"}, "", "", true},
		{"only_value", args{"value", ":"}, "", "", true},
		{"key_value", args{"key:value", ":"}, "key", "value", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := splitToKeyValue(tt.args.s, tt.args.sep)
			if (err != nil) != tt.wantErr {
				t.Errorf("splitToKeyValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("splitToKeyValue() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("splitToKeyValue() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
