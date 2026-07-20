package useragent

import "testing"

func TestClassify(t *testing.T) {
	value := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0"
	got := Classify(value)
	if got.Browser != "Edge" || got.OperatingSystem != "Windows" || got.Bot {
		t.Fatalf("Classify() = %#v", got)
	}
}

func TestClassifyBot(t *testing.T) {
	if got := Classify("ExampleBot/1.0"); !got.Bot {
		t.Fatalf("Classify() = %#v", got)
	}
}
