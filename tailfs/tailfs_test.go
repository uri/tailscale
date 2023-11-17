package tailfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitSMBConfIfNecessary(t *testing.T) {
	tmpDir := t.TempDir()
	smbConfPath := filepath.Join(tmpDir, "smb.conf")
	s := &Server{
		smbConfPath: smbConfPath,
		opts:        &Opts{StateDir: tmpDir},
	}
	err := s.initSMBConfIfNecessary()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(smbConfPath)
	if err != nil {
		t.Fatal(err)
	}
	conf := string(b)
	ds := directorySettings()
	for _, d := range ds {
		absolutePath := filepath.Join(tmpDir, d.path)
		fi, err := os.Stat(absolutePath)
		if err != nil {
			t.Fatalf("stat %v: %v", absolutePath, err)
		}
		if !fi.IsDir() {
			t.Fatalf("%v is not a directory", absolutePath)
		}
		if !strings.Contains(conf, fmt.Sprintf("%v = %v", d.Setting, absolutePath)) {
			fmt.Println(conf)
			t.Fatalf("conf does not contain %v = %v", d.Setting, absolutePath)
		}
	}
}
