package danmaku

import "testing"

func TestNormalizeDouyinRoom(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"215989414683", "215989414683", true},
		{"https://live.douyin.com/215989414683", "215989414683", true},
		{"live.douyin.com/215989414683", "215989414683", true},
		{"", "", false},
		{"bad/id", "", false},
	}
	for _, tc := range tests {
		got, err := NormalizeDouyinRoom(tc.in)
		if tc.ok && err != nil {
			t.Fatalf("%q: unexpected err %v", tc.in, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%q: expected error", tc.in)
		}
		if tc.ok && got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestDyhubWSURL(t *testing.T) {
	u, err := dyhubWSURL("http://127.0.0.1:8757", "215989414683")
	if err != nil {
		t.Fatal(err)
	}
	if u != "ws://127.0.0.1:8757/ws?roomId=215989414683&types=chat" {
		t.Fatalf("unexpected url %q", u)
	}
}
