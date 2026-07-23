package cliapi

import "testing"

func TestIsGzip(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"gzip magic", []byte{0x1f, 0x8b, 0x08, 0x00}, true},
		{"plain sql", []byte("-- MySQL dump\n"), false},
		{"empty", []byte{}, false},
		{"tek byte", []byte{0x1f}, false},
	}
	for _, c := range cases {
		if got := isGzip(c.in); got != c.want {
			t.Errorf("%s: isGzip=%v, istenen=%v", c.name, got, c.want)
		}
	}
}
