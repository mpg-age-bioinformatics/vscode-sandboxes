package startui

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRemoteTransferProgressPreservesStreamAndThrottles(t *testing.T) {
	input := bytes.Repeat([]byte("x"), 128)
	var output, log bytes.Buffer
	started := time.Now()
	clock := started
	reader := &remoteTransferReader{source: bytes.NewReader(input), log: &log, name: "image.tar", total: int64(len(input)), started: started, last: started, now: func() time.Time { clock = clock.Add(500 * time.Millisecond); return clock }}
	buffer := make([]byte, 16)
	for {
		n, err := reader.Read(buffer)
		output.Write(buffer[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(input, output.Bytes()) {
		t.Fatal("transfer data changed")
	}
	text := log.String()
	if strings.Count(text, "Upload image.tar:") != 2 || !strings.Contains(text, "100.0%") || !strings.Contains(text, "MiB/s") || !strings.Contains(text, "remaining") || !strings.Contains(text, "waiting for the remote SHA-256") {
		t.Fatal(text)
	}
}

func TestRemoteTransferProgressUsesOneSSHAndConfirmsChecksum(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX fixture")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	os.Mkdir(bin, 0700)
	fixture := `#!/usr/bin/env python3
import os,pathlib,subprocess,shlex,sys
root=pathlib.Path(os.environ['TRANSFER_FIXTURE'])
with (root/'calls').open('a') as stream:stream.write('SSH\n')
if os.environ.get('TRANSFER_FAIL'):sys.exit(1)
result=subprocess.run(shlex.split(sys.argv[-1]),input=sys.stdin.buffer.read(),stdout=subprocess.PIPE,stderr=subprocess.PIPE)
sys.stderr.buffer.write(result.stderr);sys.exit(result.returncode)
`
	os.WriteFile(filepath.Join(bin, "ssh"), []byte(fixture), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TRANSFER_FIXTURE", root)
	source := filepath.Join(root, "image.tar")
	target := filepath.Join(root, "received.tar")
	input := bytes.Repeat([]byte("binary archive\x00"), 1000)
	os.WriteFile(source, input, 0600)
	var log bytes.Buffer
	if err := uploadRemoteFileWithProgress(context.Background(), "fixture", source, target, &log); err != nil {
		t.Fatal(err)
	}
	received, _ := os.ReadFile(target)
	if !bytes.Equal(input, received) {
		t.Fatal("received archive differs")
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls"))
	if string(calls) != "SSH\n" {
		t.Fatal("progress used extra SSH requests", string(calls))
	}
	if !strings.Contains(log.String(), "remote SHA-256 verified") {
		t.Fatal(log.String())
	}
	log.Reset()
	t.Setenv("TRANSFER_FAIL", "1")
	if err := uploadRemoteFileWithProgress(context.Background(), "fixture", source, target, &log); err == nil || strings.Contains(log.String(), "remote SHA-256 verified") {
		t.Fatal("failed transfer reported success", log.String())
	}
}
