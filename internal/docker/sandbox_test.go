package docker

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"
)

func TestSandboxPath(t *testing.T) {
	for in, want := range map[string]string{"main.py": "main.py", "./a/b.txt": "a/b.txt", "a/../b": "b"} {
		if got, ok := SandboxPath(in); !ok || got != want {
			t.Errorf("%q: %q %v", in, got, ok)
		}
	}
	for _, bad := range []string{"", ".", "..", "../x", "/etc/passwd", "a/../../x"} {
		if _, ok := SandboxPath(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestReadSandboxFilesKeepsToTheLimits(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	add := func(name string, typ byte, size int) {
		_ = tw.WriteHeader(&tar.Header{Name: name, Typeflag: typ, Mode: 0o644, Size: int64(size)})
		if typ == tar.TypeReg {
			_, _ = tw.Write(bytes.Repeat([]byte("x"), size))
		}
	}
	add("./", tar.TypeDir, 0)
	add("./out/", tar.TypeDir, 0)
	add("./out/small.txt", tar.TypeReg, 10)
	add("./big.bin", tar.TypeReg, SandboxMaxFile+1)
	add("../escape", tar.TypeReg, 1)
	_ = tw.WriteHeader(&tar.Header{Name: "./link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	_ = tw.Close()
	files, skipped := readSandboxFiles(&buf)
	if len(files) != 1 || len(files["out/small.txt"]) != 10 {
		t.Errorf("files: %v", files)
	}
	if strings.Join(skipped, ",") != "big.bin" {
		t.Errorf("skipped: %v", skipped)
	}
}

func TestCappedBuffer(t *testing.T) {
	b := &cappedBuffer{max: 5}
	n, _ := b.Write([]byte("abc"))
	m, _ := b.Write([]byte("defg"))
	if b.String() != "abcde" || !b.cut || n != 3 || m != 4 {
		t.Errorf("%q cut=%v", b.String(), b.cut)
	}
}
