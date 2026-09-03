package windowslauncher

import "testing"

func TestIsUNCPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{`C:\Users\alice\project`, false},
		{`D:/work/project`, false},
		{`\\server\share\project`, true},
		{`//wsl.localhost/Ubuntu/home/alice/project`, true},
	}
	for _, test := range tests {
		if got := isUNCPath(test.path); got != test.want {
			t.Errorf("isUNCPath(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}
