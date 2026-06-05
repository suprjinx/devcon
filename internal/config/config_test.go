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

func TestImageMetadata(t *testing.T) {
	// array form, last-wins for remoteUser, env accumulates
	label := []byte(`[
		{"id":"base","remoteUser":"root","containerEnv":{"A":"1"}},
		{"id":"common-utils","remoteUser":"vscode","containerEnv":{"B":"2"}}
	]`)
	m, err := ParseImageMetadata(label)
	if err != nil {
		t.Fatal(err)
	}
	if m.RemoteUser != "vscode" {
		t.Errorf("RemoteUser = %q, want vscode", m.RemoteUser)
	}
	if m.ContainerEnv["A"] != "1" || m.ContainerEnv["B"] != "2" {
		t.Errorf("ContainerEnv = %v", m.ContainerEnv)
	}

	// json must win over metadata; metadata fills the gaps
	dc := &DevContainer{RemoteUser: "dev", ContainerEnv: map[string]string{"A": "json"}}
	dc.ApplyImageMetadata(m)
	if dc.RemoteUser != "dev" {
		t.Errorf("json remoteUser should win, got %q", dc.RemoteUser)
	}
	if dc.ContainerEnv["A"] != "json" || dc.ContainerEnv["B"] != "2" {
		t.Errorf("env merge wrong: %v", dc.ContainerEnv)
	}

	// empty / single-object / null forms
	if m, _ := ParseImageMetadata([]byte("")); m.RemoteUser != "" {
		t.Error("empty label should yield empty metadata")
	}
	if m, _ := ParseImageMetadata([]byte(`{"remoteUser":"node"}`)); m.RemoteUser != "node" {
		t.Errorf("single-object form: got %q", m.RemoteUser)
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
