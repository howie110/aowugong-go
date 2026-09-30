package config

import "testing"

func TestBlogMediaRequiresCompleteConfiguration(t *testing.T) {
	values := map[string]string{"BLOG_OSS_BUCKET": "blog-bucket"}
	if _, err := Load(func(key string) (string, bool) { v, ok := values[key]; return v, ok }); err == nil {
		t.Fatal("partial media configuration silently ignored")
	}
}
