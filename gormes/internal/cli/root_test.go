package cli

import "testing"

func TestLoadParityFixtures(t *testing.T) {
	cli, err := LoadCLISurface("../../testdata/cli_surface.json")
	if err != nil {
		t.Fatalf("load cli surface: %v", err)
	}
	if len(cli.Commands) == 0 {
		t.Fatal("expected at least one command in cli surface fixture")
	}

	docs, err := LoadDocsSurface("../../testdata/docs_surface.json")
	if err != nil {
		t.Fatalf("load docs surface: %v", err)
	}
	if len(docs.Paths) == 0 {
		t.Fatal("expected mirrored docs paths in docs surface fixture")
	}
}
