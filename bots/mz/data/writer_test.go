package data

import (
	"archive/zip"
	"encoding/binary"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/bots/mz"
	"github.com/adams-shaun/gorge/bots/mz/atomic"
	"github.com/adams-shaun/gorge/bots/mz/encode"
)

// TestNpzLayout writes two games and reads every array back: the fork's
// datasets, CSR offsets, local policy indices sorted, yes/no visits in the
// row's last two columns.
func TestNpzLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a_vs_b.npz")
	w, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	g := encode.Arrays{Order: []encode.Key{5, 9, 12}, IDs: []int32{0, 7, 8}, Values: []int32{0, 3, 0},
		EdgeChild: []int32{1, 2}, EdgeParent: []int32{0, 0}, EdgeLabel: []int32{11, 12}}
	rows := []mz.Row{
		{Graph: g, Score: 0.25, Type: atomic.Priority, IsPlayer: true, Label: 0.5,
			Visits: []mz.Visit{{Node: 12, N: 3}, {Node: 9, N: 4}}},
		{Graph: g, Score: -0.5, Type: atomic.ChooseUse, IsPlayer: true, Label: -1,
			Visits: []mz.Visit{{Node: encode.UseFalseKey, N: 2}, {Node: encode.UseTrueKey, N: 6}}},
	}
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	w.EndGame()
	if err := w.Write(rows[0]); err != nil {
		t.Fatal(err)
	}
	w.EndGame()
	if err := w.Write(mz.Row{Graph: g, Visits: []mz.Visit{{Node: 99, N: 1}}}); err == nil {
		t.Fatal("a candidate that is not a node was written")
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	arrays := readNpz(t, path)
	want := map[string]string{
		"indices":        "[0 7 8 0 7 8 0 7 8]",
		"offsets":        "[0 3 6 9]",
		"edge_offsets":   "[0 2 4 6]",
		"policy_node":    "[1 2 1 2]",
		"policy_visits":  "[4 3 4 3]",
		"policy_offsets": "[0 2 2 4]",
		"game_offsets":   "[0 2 3]",
		"row":            "[0.5 0.25 1 0 0 0 -1 -0.5 1 5 2 6 0.5 0.25 1 0 0 0] (3, 6)",
	}
	for name, v := range want {
		if got := arrays[name]; got != v {
			t.Errorf("%s = %s, want %s", name, got, v)
		}
	}
}

// readNpz renders each array of the file as "[values] shape" (shape only
// when 2-D), parsing the .npy headers it relies on.
func readNpz(t *testing.T, path string) map[string]string {
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	out := map[string]string{}
	for _, f := range z.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		if string(b[:6]) != "\x93NUMPY" || (10+int(binary.LittleEndian.Uint16(b[8:])))%64 != 0 {
			t.Fatalf("%s: bad .npy header", f.Name)
		}
		hl := int(binary.LittleEndian.Uint16(b[8:]))
		header, data := string(b[10:10+hl]), b[10+hl:]
		var vals []string
		switch {
		case strings.Contains(header, "<i4"):
			for i := 0; i < len(data); i += 4 {
				vals = append(vals, itoa(int64(int32(binary.LittleEndian.Uint32(data[i:])))))
			}
		case strings.Contains(header, "<i8"):
			for i := 0; i < len(data); i += 8 {
				vals = append(vals, itoa(int64(binary.LittleEndian.Uint64(data[i:]))))
			}
		case strings.Contains(header, "<f4"):
			for i := 0; i < len(data); i += 4 {
				vals = append(vals, ftoa(binary.LittleEndian.Uint32(data[i:])))
			}
		}
		s := "[" + strings.Join(vals, " ") + "]"
		if i := strings.Index(header, "'shape': ("); i >= 0 && strings.Contains(header[i:], ", ") && !strings.Contains(header[i:], ",)") {
			s += " " + header[i+len("'shape': "):strings.Index(header[i:], ")")+i+1]
		}
		out[strings.TrimSuffix(f.Name, ".npy")] = s
	}
	return out
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func ftoa(bits uint32) string {
	return strconv.FormatFloat(float64(math.Float32frombits(bits)), 'g', -1, 32)
}
