package blog

import "testing"

func TestFirstBodyImage(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"no image", "<p>Text only</p>", ""},
		{"first of multiple", `<p><img src="https://pic.example/first.jpg?a=1&amp;b=2"></p><img src="second.jpg">`, "https://pic.example/first.jpg?a=1&b=2"},
		{"local image", `<img alt="photo" src="/blog/assets/photo.png" />`, "/blog/assets/photo.png"},
		{"escaped example", `<pre>&lt;img src="example.png"&gt;</pre>`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstBodyImage(tc.body); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
