package ranktracking

import "testing"

func TestNormalizeConfigAndKeywords(t *testing.T) {
	config, err := NormalizeConfig(Config{Target: " Example.COM "})
	if err != nil {
		t.Fatalf("normalize config: %v", err)
	}
	if config.Target != "example.com" || config.Location != DefaultLocation || config.Language != DefaultLanguage || config.Devices != DefaultDevice || config.SERPDepth != DefaultDepth {
		t.Fatalf("unexpected config: %#v", config)
	}

	keywords, err := NormalizeKeywords([]string{" SEO   Audit ", "seo audit", "Technical SEO"})
	if err != nil {
		t.Fatalf("normalize keywords: %v", err)
	}
	if len(keywords) != 2 || keywords[0] != "SEO Audit" || keywords[1] != "Technical SEO" {
		t.Fatalf("unexpected keywords: %#v", keywords)
	}
}

func TestClassifyChange(t *testing.T) {
	position5 := 5
	position8 := 8
	cases := []struct {
		name             string
		position         *int
		previous         *int
		observed         bool
		previousObserved bool
		want             string
	}{
		{name: "not checked", observed: false, want: "not-checked"},
		{name: "uncompared", position: &position5, observed: true, want: "uncompared"},
		{name: "new", position: &position5, observed: true, previousObserved: true, want: "new"},
		{name: "lost", previous: &position5, observed: true, previousObserved: true, want: "lost"},
		{name: "improved", position: &position5, previous: &position8, observed: true, previousObserved: true, want: "improved"},
		{name: "declined", position: &position8, previous: &position5, observed: true, previousObserved: true, want: "declined"},
		{name: "stable missing", observed: true, previousObserved: true, want: "stable"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := ClassifyChange(test.position, test.previous, test.observed, test.previousObserved)
			if got != test.want {
				t.Fatalf("change = %q, want %q", got, test.want)
			}
		})
	}
}
