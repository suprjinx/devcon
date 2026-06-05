package config

import (
	"encoding/json"
	"testing"
)

func TestStripJSONC(t *testing.T) {
	in := []byte(`{
		// a line comment
		"name": "demo", /* block comment */
		"url": "http://example.com/not-a-comment", // trailing
		"build": {
			"dockerfile": "Dockerfile", // keep
		},
		"ports": [
			8080,
			9090,
		],
	}`)

	var got struct {
		Name  string `json:"name"`
		URL   string `json:"url"`
		Build struct {
			Dockerfile string `json:"dockerfile"`
		} `json:"build"`
		Ports []int `json:"ports"`
	}
	if err := json.Unmarshal(StripJSONC(in), &got); err != nil {
		t.Fatalf("unmarshal after strip: %v\ncleaned: %s", err, StripJSONC(in))
	}
	if got.Name != "demo" {
		t.Errorf("name = %q, want demo", got.Name)
	}
	if got.URL != "http://example.com/not-a-comment" {
		t.Errorf("url = %q (// inside a string must be preserved)", got.URL)
	}
	if got.Build.Dockerfile != "Dockerfile" {
		t.Errorf("dockerfile = %q", got.Build.Dockerfile)
	}
	if len(got.Ports) != 2 || got.Ports[0] != 8080 || got.Ports[1] != 9090 {
		t.Errorf("ports = %v, want [8080 9090]", got.Ports)
	}
}

func TestLifecycleAndFlexPort(t *testing.T) {
	in := []byte(`{
		"image": "alpine",
		"forwardPorts": [3000, "5432:5432"],
		"postCreateCommand": "echo hi",
		"postStartCommand": ["sh", "-c", "echo started"]
	}`)
	var dc DevContainer
	if err := json.Unmarshal(StripJSONC(in), &dc); err != nil {
		t.Fatal(err)
	}
	if len(dc.ForwardPorts) != 2 || dc.ForwardPorts[0].PublishArg() != "3000:3000" || dc.ForwardPorts[1].PublishArg() != "5432:5432" {
		t.Errorf("forwardPorts = %v", dc.ForwardPorts)
	}
	if len(dc.PostCreateCommand) != 1 || dc.PostCreateCommand[0].Shell != "echo hi" {
		t.Errorf("postCreate = %+v", dc.PostCreateCommand)
	}
	if len(dc.PostStartCommand) != 1 || len(dc.PostStartCommand[0].Argv) != 3 {
		t.Errorf("postStart = %+v", dc.PostStartCommand)
	}
}

func TestModeAndDefaults(t *testing.T) {
	cases := []struct {
		json string
		want Mode
	}{
		{`{"image":"alpine"}`, ModeImage},
		{`{"build":{"dockerfile":"Dockerfile"}}`, ModeBuild},
		{`{"dockerComposeFile":"docker-compose.yml","service":"app"}`, ModeCompose},
	}
	for _, tc := range cases {
		var dc DevContainer
		if err := json.Unmarshal([]byte(tc.json), &dc); err != nil {
			t.Fatal(err)
		}
		if got := dc.Mode(); got != tc.want {
			t.Errorf("Mode(%s) = %v, want %v", tc.json, got, tc.want)
		}
	}
}
