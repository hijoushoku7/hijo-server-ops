package javaenv

import "testing"

func TestParseUnsupportedClassVersion(t *testing.T) {
	tests := []struct {
		name string
		log  string
		want ClassVersionError
		ok   bool
	}{
		{"実例", "UnsupportedClassVersionError: X has been compiled by a more recent version of the Java Runtime (class file version 65.0), this version of the Java Runtime only recognizes class file versions up to 61.0", ClassVersionError{21, 17}, true},
		{"複数行", "class file version 69.0\nanything\nclass file versions up to 65.0", ClassVersionError{25, 21}, true},
		{"要求のみ", "class file version 61.0", ClassVersionError{17, 0}, true},
		{"要求より前の別エラーを混ぜない", "class file versions up to 52.0\nclass file version 65.0", ClassVersionError{21, 0}, true},
		{"無関係", "UnsupportedClassVersionError only", ClassVersionError{}, false},
		{"壊れた数字", "class file version x.0", ClassVersionError{}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := ParseUnsupportedClassVersion(test.log)
			if got != test.want || ok != test.ok {
				t.Fatalf("got = %#v, %v; want %#v, %v", got, ok, test.want, test.ok)
			}
		})
	}
}
