package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"

	"github.com/usadamasa/go-arch-metrics/internal/tracked"
)

// target は測定の対象。パッケージを引数で受けるツールには packages を渡し、追跡パッケージの中で
// git が ignore するファイルの指摘はツールの出力から除く。型検査はそのファイルを含めてツールに任せる
type target struct {
	root     string
	selected tracked.Selection
}

func (t target) packages() []string { return t.selected.Patterns() }

// ignored は path を git が ignore するかを返す。相対パスはモジュールルートから解決する
func (t target) ignored(path string) bool {
	if path == "" {
		return false
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.root, path)
	}
	return t.selected.Ignored(path)
}

// dropEntries は obj[key] の配列から、file が返すパスを git が ignore する要素を除き、残った数と除いた数を返す。
// 1 つも除かなければ obj を書き換えない
func (t target) dropEntries(obj map[string]json.RawMessage, key string, file func(json.RawMessage) string) (kept, dropped int, err error) {
	var entries []json.RawMessage
	if raw, ok := obj[key]; ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return 0, 0, err
		}
	}
	remaining := make([]json.RawMessage, 0, len(entries))
	for _, e := range entries {
		if !t.ignored(file(e)) {
			remaining = append(remaining, e)
		}
	}
	dropped = len(entries) - len(remaining)
	if dropped == 0 {
		return len(remaining), 0, nil
	}
	if obj[key], err = marshal(remaining); err != nil {
		return 0, 0, err
	}
	return len(remaining), dropped, nil
}

// marshal は writeJSON と同じく HTML の文字をエスケープせずに書く。json.Marshal は RawMessage の中身まで
// エスケープし直すので使わない
func marshal(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// stringAt は JSON object を keys の順にたどった文字列を返す。たどれなければ空
func stringAt(keys ...string) func(json.RawMessage) string {
	return func(raw json.RawMessage) string {
		for _, k := range keys {
			var obj map[string]json.RawMessage
			if json.Unmarshal(raw, &obj) != nil {
				return ""
			}
			raw = obj[k]
		}
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return ""
		}
		return s
	}
}

// filterGolangci は golangci-lint の Issues から ignore されたファイルの指摘を除く。Pos.Filename は相対パスで出る
func (t target) filterGolangci(raw json.RawMessage) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if _, dropped, err := t.dropEntries(obj, "Issues", stringAt("Pos", "Filename")); err != nil || dropped == 0 {
		return raw, err
	}
	return marshal(obj)
}

// filterGosec は gosec の Issues から ignore されたファイルの指摘を除き、Stats.found を残った数にそろえる。
// Stats の files と lines は ignore されたファイルを含んだまま
func (t target) filterGosec(raw json.RawMessage) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	kept, dropped, err := t.dropEntries(obj, "Issues", stringAt("file"))
	if err != nil || dropped == 0 {
		return raw, err
	}
	var stats map[string]json.RawMessage
	if err := json.Unmarshal(obj["Stats"], &stats); err != nil {
		return nil, err
	}
	if stats["found"], err = json.Marshal(kept); err != nil {
		return nil, err
	}
	if obj["Stats"], err = marshal(stats); err != nil {
		return nil, err
	}
	return marshal(obj)
}

// filterGoArchLint は go-arch-lint の 3 種の違反から ignore されたファイルのものを除き、ArchHasWarnings を
// 残った違反の有無にそろえる。go-arch-lint はパッケージを引数で受けないので、ignore されたディレクトリもここで除く
func (t target) filterGoArchLint(raw json.RawMessage) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(obj["Payload"], &payload); err != nil {
		return nil, err
	}
	warnings := []struct {
		key  string
		file func(json.RawMessage) string
	}{
		{"ArchWarningsDeps", stringAt("FileAbsolutePath")},
		{"ArchWarningsNotMatched", stringAt("FileAbsolutePath")},
		{"ArchWarningsDeepScan", stringAt("Dependency", "Injection", "File")},
	}
	var totalKept, totalDropped int
	for _, w := range warnings {
		kept, dropped, err := t.dropEntries(payload, w.key, w.file)
		if err != nil {
			return nil, err
		}
		totalKept += kept
		totalDropped += dropped
	}
	if totalDropped == 0 {
		return raw, nil
	}
	var err error
	if payload["ArchHasWarnings"], err = json.Marshal(totalKept > 0); err != nil {
		return nil, err
	}
	if obj["Payload"], err = marshal(payload); err != nil {
		return nil, err
	}
	return marshal(obj)
}
