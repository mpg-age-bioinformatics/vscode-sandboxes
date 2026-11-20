package startui

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRemoteSlurmRelayBinaryStream(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash and Python")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python unavailable")
	}
	for _, modern := range []bool{false, true} {
		t.Run(strconv.FormatBool(modern), func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			os.Mkdir(bin, 0700)
			token := strings.Repeat("a", 32)
			for name, data := range map[string]string{"owner-token": token, "job-id": "123", "modules.sh": "echo module-message >&2\n", "image": "fixture.sif", "apptainer": filepath.Join(bin, "apptainer")} {
				os.WriteFile(filepath.Join(root, name), []byte(data), 0600)
			}
			help := ""
			if modern {
				help = "--overlap --exact"
			}
			os.WriteFile(filepath.Join(bin, "squeue"), []byte("#!/bin/bash\nprintf '123|vscode-sandboxes:"+token+"\\n'\n"), 0700)
			os.WriteFile(filepath.Join(bin, "srun"), []byte("#!/bin/bash\nif [[ $1 == --help ]]; then echo '"+help+"'; exit; fi\nprintf '%s\\n' \"$@\" > \"$RELAY_ARGS\"\nwhile [[ $1 == --* ]]; do shift; done\nexec \"$@\"\n"), 0700)
			os.WriteFile(filepath.Join(bin, "apptainer"), []byte("#!/bin/bash\nwhile [[ $1 != fixture.sif ]]; do shift; done\nshift\nexec \"$@\"\n"), 0700)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+filepath.Dir(python)+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("RELAY_ARGS", filepath.Join(root, "srun-args"))
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
			os.WriteFile(filepath.Join(root, "ready"), []byte("PORT="+port+"\n"), 0600)
			go func() {
				conn, e := listener.Accept()
				if e == nil {
					defer conn.Close()
					io.Copy(conn, conn)
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			pattern := make([]byte, 256)
			for i := range pattern {
				pattern[i] = byte(i)
			}
			input := bytes.Repeat(pattern, 4096)
			script := filepath.Join(root, "relay.sh")
			os.WriteFile(script, []byte(remoteAsset("relay.sh")), 0600)
			cmd := exec.CommandContext(ctx, "bash", script, root, token, "123", port)
			cmd.Stdin = bytes.NewReader(input)
			var out, stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatal(err, stderr.String())
			}
			if !bytes.Equal(input, out.Bytes()) {
				t.Fatalf("binary stream corrupted: %d != %d bytes", len(input), out.Len())
			}
			args, _ := os.ReadFile(filepath.Join(root, "srun-args"))
			if strings.Contains(string(args), "--overlap") != modern || strings.Contains(string(args), "--pty") || !strings.Contains(string(args), "--unbuffered") {
				t.Fatal(string(args))
			}
			os.Remove(filepath.Join(root, "srun-args"))
			bad := exec.CommandContext(ctx, "bash", script, root, "wrong-token", "123", port)
			if bad.Run() == nil {
				t.Fatal("accepted wrong allocation identity")
			}
			if _, err := os.Stat(filepath.Join(root, "srun-args")); !os.IsNotExist(err) {
				t.Fatal("launched a step for mismatched identity")
			}
		})
	}
}
