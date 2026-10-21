// Package data is the mage fork's LabeledStateWriter: it writes training
// rows in the fork's layout, every dataset CSR-indexed by its offsets, one
// row per searched state. The file is a NumPy .npz (a stored zip of .npy
// arrays), not HDF5: the one departure from principle 6 (mz-gorge-design.md).
//
// Layout:
//
//	indices        int32 [nodes]       node feature id (Node.id)
//	values         int32 [nodes]       node numeric value
//	offsets        int64 [N+1]         node offsets
//	edge_child     int32 [edges]       local index of the child
//	edge_parent    int32 [edges]       local index of the parent
//	edge_label     int32 [edges]       hashed edge label
//	edge_offsets   int64 [N+1]         edge offsets
//	policy_node    int32 [candidates]  local index of the candidate node
//	policy_visits  int32 [candidates]  MCTS visit count
//	policy_offsets int64 [N+1]         candidate offsets
//	row            float32 [N, 6]      resultLabel, stateScore, isPlayer, actionType, useFalseVisits, useTrueVisits
//	game_offsets   int64 [G+1]         row index where each game starts
package data

import (
	"archive/zip"
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/adams-shaun/gorge/bots/mz"
	"github.com/adams-shaun/gorge/bots/mz/atomic"
	"github.com/adams-shaun/gorge/bots/mz/encode"
)

// A Writer writes one .npz file of training rows.
type Writer struct {
	path   string
	sets   []*dataset
	byName map[string]*dataset

	rows, games, nodes, edges, candidates int64
}

// A dataset is one array, streamed to a temporary file until Close.
type dataset struct {
	name, descr string
	width       int // columns of a 2-D array, else 0
	f           *os.File
	w           *bufio.Writer
	n           int64 // elements written
	buf         [8]byte
}

var layout = []struct {
	name, descr string
	width       int
}{
	{"indices", "<i4", 0}, {"values", "<i4", 0}, {"offsets", "<i8", 0},
	{"edge_child", "<i4", 0}, {"edge_parent", "<i4", 0}, {"edge_label", "<i4", 0}, {"edge_offsets", "<i8", 0},
	{"policy_node", "<i4", 0}, {"policy_visits", "<i4", 0}, {"policy_offsets", "<i8", 0},
	{"row", "<f4", 6}, {"game_offsets", "<i8", 0},
}

// Create starts the file at path.
func Create(path string) (*Writer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	w := &Writer{path: path, byName: map[string]*dataset{}}
	for _, l := range layout {
		f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"."+l.name+".*")
		if err != nil {
			w.discard()
			return nil, err
		}
		d := &dataset{name: l.name, descr: l.descr, width: l.width, f: f, w: bufio.NewWriter(f)}
		w.sets = append(w.sets, d)
		w.byName[l.name] = d
	}
	for _, name := range []string{"offsets", "edge_offsets", "policy_offsets", "game_offsets"} {
		w.byName[name].i64(0)
	}
	return w, nil
}

// actionTypes are ActionEncoder.ActionType's ordinals.
var actionTypes = map[atomic.Type]float32{
	atomic.Priority: 0, atomic.ChooseNum: 1, atomic.ChooseTarget: 3, atomic.MakeChoice: 4, atomic.ChooseUse: 5,
}

// Write appends one row (writeRecord). Every candidate must be a node of
// the row's graph, but a yes/no's, which go in the row's last two columns.
func (w *Writer) Write(r mz.Row) error {
	g := r.Graph
	local := make(map[encode.Key]int32, len(g.Order))
	for i, k := range g.Order {
		local[k] = int32(i)
	}
	var useFalse, useTrue float32
	var cands [][2]int32
	for _, v := range r.Visits {
		switch v.Node {
		case encode.UseFalseKey:
			useFalse = float32(v.N)
		case encode.UseTrueKey:
			useTrue = float32(v.N)
		default:
			i, ok := local[v.Node]
			if !ok {
				return fmt.Errorf("data: policy candidate %x is not a graph node", v.Node)
			}
			cands = append(cands, [2]int32{i, int32(v.N)})
		}
	}
	slices.SortFunc(cands, func(a, b [2]int32) int { return int(a[0] - b[0]) })

	w.byName["indices"].i32s(g.IDs)
	w.byName["values"].i32s(g.Values)
	w.nodes += int64(len(g.IDs))
	w.byName["offsets"].i64(w.nodes)
	w.byName["edge_child"].i32s(g.EdgeChild)
	w.byName["edge_parent"].i32s(g.EdgeParent)
	w.byName["edge_label"].i32s(g.EdgeLabel)
	w.edges += int64(len(g.EdgeChild))
	w.byName["edge_offsets"].i64(w.edges)
	for _, c := range cands {
		w.byName["policy_node"].i32(c[0])
		w.byName["policy_visits"].i32(c[1])
	}
	w.candidates += int64(len(cands))
	w.byName["policy_offsets"].i64(w.candidates)
	isPlayer := float32(0)
	if r.IsPlayer {
		isPlayer = 1
	}
	for _, f := range []float32{float32(r.Label), float32(r.Score), isPlayer, actionTypes[r.Type], useFalse, useTrue} {
		w.byName["row"].f32(f)
	}
	w.rows++
	return nil
}

// EndGame marks where the next game's rows start.
func (w *Writer) EndGame() {
	w.byName["game_offsets"].i64(w.rows)
	w.games++
}

// Rows is the number of rows written.
func (w *Writer) Rows() int64 { return w.rows }

// Close writes the .npz and removes the temporary files.
func (w *Writer) Close() error {
	defer w.discard()
	out, err := os.Create(w.path)
	if err != nil {
		return err
	}
	z := zip.NewWriter(out)
	for _, d := range w.sets {
		if err := d.writeNpy(z); err != nil {
			out.Close()
			return err
		}
	}
	if err := z.Close(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func (w *Writer) discard() {
	for _, d := range w.sets {
		d.f.Close()
		os.Remove(d.f.Name())
	}
}

func (d *dataset) i32(v int32) {
	d.w.Write(binary.LittleEndian.AppendUint32(d.buf[:0], uint32(v)))
	d.n++
}

func (d *dataset) i32s(vs []int32) {
	for _, v := range vs {
		d.i32(v)
	}
}

func (d *dataset) i64(v int64) {
	d.w.Write(binary.LittleEndian.AppendUint64(d.buf[:0], uint64(v)))
	d.n++
}

func (d *dataset) f32(v float32) {
	d.w.Write(binary.LittleEndian.AppendUint32(d.buf[:0], math.Float32bits(v)))
	d.n++
}

// writeNpy writes the dataset as one .npy entry (format 1.0) of the zip.
func (d *dataset) writeNpy(z *zip.Writer) error {
	if err := d.w.Flush(); err != nil {
		return err
	}
	shape := fmt.Sprintf("(%d,)", d.n)
	if d.width > 0 {
		shape = fmt.Sprintf("(%d, %d)", d.n/int64(d.width), d.width)
	}
	header := fmt.Sprintf("{'descr': '%s', 'fortran_order': False, 'shape': %s, }", d.descr, shape)
	// magic (6) + version (2) + length (2) + header + '\n', padded to 64.
	pad := 64 - (10+len(header)+1)%64
	header += strings.Repeat(" ", pad%64) + "\n"
	e, err := z.CreateHeader(&zip.FileHeader{Name: d.name + ".npy", Method: zip.Store})
	if err != nil {
		return err
	}
	pre := append([]byte("\x93NUMPY\x01\x00"), byte(len(header)), byte(len(header)>>8))
	if _, err := e.Write(append(pre, header...)); err != nil {
		return err
	}
	if _, err := d.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err = io.Copy(e, d.f)
	return err
}
