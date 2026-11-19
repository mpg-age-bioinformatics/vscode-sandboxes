package startui

import "testing"

func TestUnifiedTypeSelection(t *testing.T) {
	c := Config{Sandboxes: Catalog()}
	for _, item := range Catalog() {
		s := selection()
		s.Sandbox = item.Slug
		s.Version = item.Version
		got, err := resolveSelection(c, s)
		if err != nil || got.Sandbox != item.Slug {
			t.Fatalf("%s: %v %v", item.Slug, got, err)
		}
		if item.Slug == "bioinformatics-sandbox" && got.Version != "" {
			t.Fatal("general sandbox has a language version")
		}
	}
	s := selection()
	s.Sandbox = "../../python-sandbox"
	s.Version = "3.13"
	if _, err := resolveSelection(c, s); err == nil {
		t.Fatal("accepted unknown type")
	}
	s.Sandbox = "r-sandbox"
	s.Version = "3.x"
	if _, err := resolveSelection(c, s); err == nil {
		t.Fatal("accepted invalid R version")
	}
}
func TestProjectTypeCannotBeChanged(t *testing.T) {
	c := Config{Slug: "python-sandbox", ProjectMode: true}
	s := selection()
	s.Sandbox = "r-sandbox"
	if _, err := resolveSelection(c, s); err == nil {
		t.Fatal("changed a fixed project type")
	}
	s.Sandbox = ""
	s.Version = ""
	got, err := resolveSelection(c, s)
	if err != nil || got.Sandbox != "python-sandbox" {
		t.Fatal(got, err)
	}
}

func TestBothAgentsReplaceSavedChoiceAndAllowPrivateCodex(t *testing.T) {
	for _, previous := range []string{"", "codex", "claude", "both"} {
		s := selection()
		s.Sandbox, s.Agent = "python-sandbox", previous
		s.Endpoint, s.BaseURL, s.Model = "private", "https://models.example.org/v1", "private-model"
		s.Token = "sandbox-key"
		got, err := resolveSelection(Config{Sandboxes: Catalog()}, s)
		if err != nil || got.Agent != "both" {
			t.Fatalf("saved choice %q: %v %v", previous, got, err)
		}
		if err := Validate(got); err != nil {
			t.Fatal(err)
		}
	}
}
