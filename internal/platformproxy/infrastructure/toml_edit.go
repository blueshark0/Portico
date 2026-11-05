package infrastructure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

type tomlFieldSpec struct {
	Key  string
	Path []string
}

type tomlFieldValue struct {
	Present bool
	Raw     string
}

type tomlEditRange struct {
	start int
	end   int
}

type tomlEditKey struct {
	text   string
	source tomlEditRange
}

type tomlEditValue struct {
	kind     unstable.Kind
	source   tomlEditRange
	children []*tomlEditKeyValue
}

type tomlEditKeyValue struct {
	keys   []tomlEditKey
	value  *tomlEditValue
	source tomlEditRange
	path   []string
	scope  *tomlEditScope
}

type tomlEditExpression struct {
	kind   unstable.Kind
	keys   []tomlEditKey
	kv     *tomlEditKeyValue
	source tomlEditRange
}

type tomlEditScope struct {
	kind     unstable.Kind
	path     []string
	expr     *tomlEditExpression
	value    *tomlEditValue
	bindings []*tomlEditKeyValue
}

type tomlEditDocument struct {
	data      []byte
	semantic  map[string]any
	root      *tomlEditScope
	exprs     []*tomlEditExpression
	scopes    []*tomlEditScope
	bindings  []*tomlEditKeyValue
	arrayPath [][]string
}

// readTOMLFieldValues locates managed scalar values without normalizing their
// spelling. In particular, strings retain their quotes in Raw.
func readTOMLFieldValues(data []byte, specs []tomlFieldSpec) (map[string]tomlFieldValue, error) {
	doc, err := parseTOMLEditDocument(data)
	if err != nil {
		return nil, err
	}

	result := make(map[string]tomlFieldValue, len(specs))
	seenKeys := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if len(spec.Path) == 0 {
			return nil, fmt.Errorf("toml field %q: empty path", spec.Key)
		}
		if _, exists := seenKeys[spec.Key]; exists {
			return nil, fmt.Errorf("toml field %q: duplicate result key", spec.Key)
		}
		seenKeys[spec.Key] = struct{}{}

		if err := doc.checkParentContainers(spec.Path); err != nil {
			return nil, err
		}
		value, found := doc.lookup(spec.Path)
		matches := doc.bindingsAt(spec.Path)
		if len(matches) > 1 {
			return nil, fmt.Errorf("toml field %s: duplicate target", formatTOMLPath(spec.Path))
		}
		if !found {
			if len(matches) != 0 {
				return nil, fmt.Errorf("toml field %s: cannot safely locate value", formatTOMLPath(spec.Path))
			}
			result[spec.Key] = tomlFieldValue{}
			continue
		}
		if isTOMLContainerValue(value) {
			return nil, fmt.Errorf("toml field %s: target is not a scalar", formatTOMLPath(spec.Path))
		}
		if len(matches) != 1 || !isTOMLScalarKind(matches[0].value.kind) {
			return nil, fmt.Errorf("toml field %s: cannot safely locate scalar", formatTOMLPath(spec.Path))
		}
		raw := matches[0].value.source
		result[spec.Key] = tomlFieldValue{Present: true, Raw: string(data[raw.start:raw.end])}
	}
	return result, nil
}

// applyTOMLFieldEdit applies one minimal source patch. The input is never
// modified, including when validation of the candidate document fails.
func applyTOMLFieldEdit(data []byte, path []string, raw string, present bool) ([]byte, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("toml edit: empty path")
	}
	doc, err := parseTOMLEditDocument(data)
	if err != nil {
		return nil, err
	}
	if err := doc.checkParentContainers(path); err != nil {
		return nil, err
	}

	if !present {
		candidate, changed, err := doc.deleteField(path)
		if err != nil {
			return nil, err
		}
		if !changed {
			return data, nil
		}
		return validateTOMLEditCandidate(candidate)
	}

	newValue, err := parseTOMLEditLiteral(raw)
	if err != nil {
		return nil, fmt.Errorf("toml edit %s: invalid literal: %w", formatTOMLPath(path), err)
	}

	oldValue, found := doc.lookup(path)
	matches := doc.bindingsAt(path)
	if len(matches) > 1 {
		return nil, fmt.Errorf("toml edit %s: duplicate target", formatTOMLPath(path))
	}
	if found {
		if isTOMLContainerValue(oldValue) {
			return nil, fmt.Errorf("toml edit %s: target is a container", formatTOMLPath(path))
		}
		if len(matches) != 1 || !isTOMLScalarKind(matches[0].value.kind) {
			return nil, fmt.Errorf("toml edit %s: cannot safely locate scalar", formatTOMLPath(path))
		}
		source := matches[0].value.source
		oldRaw := string(data[source.start:source.end])
		if oldRaw == raw || reflect.DeepEqual(oldValue, newValue) {
			return data, nil
		}
		candidate := patchTOMLBytes(data, source.start, source.end, []byte(raw))
		return validateTOMLEditCandidate(candidate)
	}
	if len(matches) != 0 {
		return nil, fmt.Errorf("toml edit %s: cannot safely locate insertion", formatTOMLPath(path))
	}

	candidate, err := doc.insertField(path, raw)
	if err != nil {
		return nil, err
	}
	return validateTOMLEditCandidate(candidate)
}

// pruneTOMLContainers removes only explicitly represented, semantically empty
// standard tables or inline-table assignments. Implicit tables have no source
// of their own to remove. Paths are applied in order, so child paths should be
// supplied before their parents.
func pruneTOMLContainers(data []byte, paths [][]string) ([]byte, error) {
	current := data
	changedAny := false
	for _, path := range paths {
		if len(path) == 0 {
			return nil, fmt.Errorf("toml prune: empty path")
		}
		doc, err := parseTOMLEditDocument(current)
		if err != nil {
			return nil, err
		}
		if err := doc.checkParentContainers(path); err != nil {
			return nil, err
		}
		value, found := doc.lookup(path)
		if !found || !isTOMLMap(value) || len(value.(map[string]any)) != 0 {
			continue
		}

		scopes := doc.scopesAt(path)
		if len(scopes) > 1 {
			return nil, fmt.Errorf("toml prune %s: duplicate container", formatTOMLPath(path))
		}
		if len(scopes) == 0 {
			continue
		}
		scope := scopes[0]
		var candidate []byte
		switch scope.kind {
		case unstable.Table:
			candidate = deleteTOMLLine(current, scope.expr.source)
		case unstable.InlineTable:
			matches := doc.bindingsAt(path)
			if len(matches) != 1 || matches[0].value != scope.value {
				return nil, fmt.Errorf("toml prune %s: cannot safely locate inline assignment", formatTOMLPath(path))
			}
			candidate = doc.deleteKeyValue(matches[0])
		default:
			continue
		}
		validated, err := validateTOMLEditCandidate(candidate)
		if err != nil {
			return nil, err
		}
		current = validated
		changedAny = true
	}
	if !changedAny {
		if _, err := parseTOMLEditDocument(data); err != nil {
			return nil, err
		}
		return data, nil
	}
	return current, nil
}

func parseTOMLEditDocument(data []byte) (*tomlEditDocument, error) {
	semantic := make(map[string]any)
	if err := toml.Unmarshal(data, &semantic); err != nil {
		return nil, fmt.Errorf("invalid TOML: %w", err)
	}

	doc := &tomlEditDocument{data: data, semantic: semantic}
	doc.root = &tomlEditScope{kind: unstable.Invalid}
	doc.scopes = append(doc.scopes, doc.root)
	current := doc.root

	parser := unstable.Parser{}
	parser.Reset(data)
	for parser.NextExpression() {
		node := parser.Expression()
		expr := &tomlEditExpression{kind: node.Kind}
		switch node.Kind {
		case unstable.Table, unstable.ArrayTable:
			keys, err := copyTOMLEditKeys(&parser, node.Key())
			if err != nil {
				return nil, err
			}
			expr.keys = keys
			end, err := tomlTableHeaderEnd(data, keys, node.Kind)
			if err != nil {
				return nil, err
			}
			expr.source = tomlEditRange{start: keys[0].source.start, end: end}
			doc.exprs = append(doc.exprs, expr)
			path := tomlEditKeyTexts(keys)
			if node.Kind == unstable.ArrayTable {
				doc.arrayPath = append(doc.arrayPath, append([]string(nil), path...))
				current = nil
				continue
			}
			if tomlPathInsideArray(doc.arrayPath, path) {
				current = nil
				continue
			}
			scope := &tomlEditScope{kind: unstable.Table, path: path, expr: expr}
			doc.scopes = append(doc.scopes, scope)
			current = scope
		case unstable.KeyValue:
			kv, err := copyTOMLEditKeyValue(&parser, node)
			if err != nil {
				return nil, err
			}
			expr.kv = kv
			expr.source = kv.source
			doc.exprs = append(doc.exprs, expr)
			if current != nil {
				doc.indexKeyValue(current, kv)
			}
		default:
			return nil, fmt.Errorf("invalid TOML: unsupported expression kind %s", node.Kind)
		}
	}
	if err := parser.Error(); err != nil {
		return nil, fmt.Errorf("invalid TOML: %w", err)
	}
	return doc, nil
}

func copyTOMLEditKeys(parser *unstable.Parser, iterator unstable.Iterator) ([]tomlEditKey, error) {
	var keys []tomlEditKey
	for iterator.Next() {
		node := iterator.Node()
		if node.Kind != unstable.Key {
			return nil, fmt.Errorf("invalid TOML: expected key node")
		}
		source, err := tomlEditRangeFromParser(parser, node.Raw)
		if err != nil {
			return nil, err
		}
		keys = append(keys, tomlEditKey{text: string(node.Data), source: source})
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("invalid TOML: empty key")
	}
	return keys, nil
}

func copyTOMLEditKeyValue(parser *unstable.Parser, node *unstable.Node) (*tomlEditKeyValue, error) {
	keys, err := copyTOMLEditKeys(parser, node.Key())
	if err != nil {
		return nil, err
	}
	valueStart, err := tomlValueStart(parser.Data(), keys[len(keys)-1].source.end)
	if err != nil {
		return nil, err
	}
	value, err := copyTOMLEditValue(parser, node.Value(), valueStart)
	if err != nil {
		return nil, err
	}
	return &tomlEditKeyValue{
		keys:   keys,
		value:  value,
		source: tomlEditRange{start: keys[0].source.start, end: value.source.end},
	}, nil
}

func copyTOMLEditValue(parser *unstable.Parser, node *unstable.Node, valueStart int) (*tomlEditValue, error) {
	value := &tomlEditValue{kind: node.Kind}
	data := parser.Data()
	if node.Kind == unstable.InlineTable || node.Kind == unstable.Array {
		end, err := scanTOMLCompoundEnd(data, valueStart)
		if err != nil {
			return nil, err
		}
		value.source = tomlEditRange{start: valueStart, end: end}
	} else {
		raw := node.Raw
		if node.Kind != unstable.String && raw.Length == 0 {
			// Bool and date/time nodes do not consistently populate Raw. Data is
			// a subslice of Parser.Data for these scalar kinds.
			raw = parser.Range(node.Data)
		}
		source, err := tomlEditRangeFromParser(parser, raw)
		if err != nil {
			return nil, err
		}
		if source.start != valueStart {
			return nil, fmt.Errorf("invalid TOML: inconsistent value location")
		}
		value.source = source
	}

	if node.Kind == unstable.InlineTable {
		children := node.Children()
		for children.Next() {
			child := children.Node()
			if child.Kind != unstable.KeyValue {
				return nil, fmt.Errorf("invalid TOML: non-key inline child")
			}
			kv, err := copyTOMLEditKeyValue(parser, child)
			if err != nil {
				return nil, err
			}
			value.children = append(value.children, kv)
		}
	}
	return value, nil
}

func tomlEditRangeFromParser(parser *unstable.Parser, raw unstable.Range) (tomlEditRange, error) {
	start := int(raw.Offset)
	end := start + int(raw.Length)
	if start < 0 || end < start || end > len(parser.Data()) {
		return tomlEditRange{}, fmt.Errorf("invalid TOML: source range out of bounds")
	}
	return tomlEditRange{start: start, end: end}, nil
}

func tomlValueStart(data []byte, offset int) (int, error) {
	i := offset
	for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
		i++
	}
	if i >= len(data) || data[i] != '=' {
		return 0, fmt.Errorf("invalid TOML: expected equals sign")
	}
	i++
	for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
		i++
	}
	if i >= len(data) {
		return 0, fmt.Errorf("invalid TOML: missing value")
	}
	return i, nil
}

func tomlTableHeaderEnd(data []byte, keys []tomlEditKey, kind unstable.Kind) (int, error) {
	i := keys[len(keys)-1].source.end
	for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
		i++
	}
	closing := 1
	if kind == unstable.ArrayTable {
		closing = 2
	}
	if i+closing > len(data) || !bytes.Equal(data[i:i+closing], bytes.Repeat([]byte{']'}, closing)) {
		return 0, fmt.Errorf("invalid TOML: unterminated table header")
	}
	return i + closing, nil
}

// scanTOMLCompoundEnd finds a compound value's closing delimiter. The parser
// exposes the opening brace for inline tables but not their full Range.
func scanTOMLCompoundEnd(data []byte, start int) (int, error) {
	if start >= len(data) || (data[start] != '{' && data[start] != '[') {
		return 0, fmt.Errorf("invalid TOML: invalid compound value start")
	}
	stack := make([]byte, 0, 4)
	for i := start; i < len(data); {
		switch data[i] {
		case '\'', '"':
			end, err := scanTOMLStringEnd(data, i)
			if err != nil {
				return 0, err
			}
			i = end
		case '#':
			for i < len(data) && data[i] != '\n' {
				i++
			}
		case '{':
			stack = append(stack, '}')
			i++
		case '[':
			stack = append(stack, ']')
			i++
		case '}', ']':
			if len(stack) == 0 || data[i] != stack[len(stack)-1] {
				return 0, fmt.Errorf("invalid TOML: mismatched compound delimiter")
			}
			stack = stack[:len(stack)-1]
			i++
			if len(stack) == 0 {
				return i, nil
			}
		default:
			i++
		}
	}
	return 0, fmt.Errorf("invalid TOML: unterminated compound value")
}

func scanTOMLStringEnd(data []byte, start int) (int, error) {
	quote := data[start]
	triple := start+2 < len(data) && data[start+1] == quote && data[start+2] == quote
	if triple {
		for i := start + 3; i < len(data); {
			if data[i] == '\\' && quote == '"' {
				i += 2
				continue
			}
			if data[i] != quote {
				i++
				continue
			}
			j := i
			for j < len(data) && data[j] == quote {
				j++
			}
			if j-i >= 3 {
				return j, nil
			}
			i = j
		}
		return 0, fmt.Errorf("invalid TOML: unterminated multiline string")
	}
	for i := start + 1; i < len(data); i++ {
		if data[i] == '\\' && quote == '"' {
			i++
			continue
		}
		if data[i] == quote {
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("invalid TOML: unterminated string")
}

func (doc *tomlEditDocument) indexKeyValue(scope *tomlEditScope, kv *tomlEditKeyValue) {
	kv.scope = scope
	kv.path = appendTOMLPath(scope.path, tomlEditKeyTexts(kv.keys)...)
	scope.bindings = append(scope.bindings, kv)
	doc.bindings = append(doc.bindings, kv)
	if kv.value.kind != unstable.InlineTable {
		return
	}
	childScope := &tomlEditScope{kind: unstable.InlineTable, path: append([]string(nil), kv.path...), value: kv.value}
	doc.scopes = append(doc.scopes, childScope)
	for _, child := range kv.value.children {
		doc.indexKeyValue(childScope, child)
	}
}

func (doc *tomlEditDocument) lookup(path []string) (any, bool) {
	var current any = doc.semantic
	for _, key := range path {
		container, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = container[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func (doc *tomlEditDocument) checkParentContainers(path []string) error {
	for i := 1; i < len(path); i++ {
		value, found := doc.lookup(path[:i])
		if found && !isTOMLMap(value) {
			return fmt.Errorf("toml path %s: parent %s is not a table", formatTOMLPath(path), formatTOMLPath(path[:i]))
		}
	}
	return nil
}

func (doc *tomlEditDocument) bindingsAt(path []string) []*tomlEditKeyValue {
	var matches []*tomlEditKeyValue
	for _, binding := range doc.bindings {
		if tomlPathsEqual(binding.path, path) {
			matches = append(matches, binding)
		}
	}
	return matches
}

func (doc *tomlEditDocument) scopesAt(path []string) []*tomlEditScope {
	var matches []*tomlEditScope
	for _, scope := range doc.scopes {
		if len(scope.path) == 0 && scope.kind == unstable.Invalid {
			continue
		}
		if tomlPathsEqual(scope.path, path) {
			matches = append(matches, scope)
		}
	}
	return matches
}

func (doc *tomlEditDocument) deleteField(path []string) ([]byte, bool, error) {
	value, found := doc.lookup(path)
	matches := doc.bindingsAt(path)
	if len(matches) > 1 {
		return nil, false, fmt.Errorf("toml delete %s: duplicate target", formatTOMLPath(path))
	}
	if !found {
		if len(matches) != 0 {
			return nil, false, fmt.Errorf("toml delete %s: cannot safely locate value", formatTOMLPath(path))
		}
		return doc.data, false, nil
	}
	if isTOMLContainerValue(value) {
		return nil, false, fmt.Errorf("toml delete %s: target is a container", formatTOMLPath(path))
	}
	if len(matches) != 1 || !isTOMLScalarKind(matches[0].value.kind) {
		return nil, false, fmt.Errorf("toml delete %s: cannot safely locate scalar", formatTOMLPath(path))
	}
	return doc.deleteKeyValue(matches[0]), true, nil
}

func (doc *tomlEditDocument) deleteKeyValue(kv *tomlEditKeyValue) []byte {
	if kv.scope.kind != unstable.InlineTable {
		return deleteTOMLLine(doc.data, kv.source)
	}
	return doc.deleteInlineKeyValue(kv)
}

func (doc *tomlEditDocument) deleteInlineKeyValue(kv *tomlEditKeyValue) []byte {
	children := kv.scope.value.children
	index := -1
	for i, child := range children {
		if child == kv {
			index = i
			break
		}
	}
	if index < 0 {
		return doc.data
	}
	if len(children) == 1 {
		return patchTOMLBytes(doc.data, kv.source.start, kv.source.end, nil)
	}
	if index+1 < len(children) {
		// Removing the first item together with its following separator keeps
		// the whitespace that originally preceded the old first item.
		return patchTOMLBytes(doc.data, kv.source.start, children[index+1].source.start, nil)
	}
	// For a later item, remove its preceding separator and the item itself.
	return patchTOMLBytes(doc.data, children[index-1].value.source.end, kv.source.end, nil)
}

func (doc *tomlEditDocument) insertField(path []string, raw string) ([]byte, error) {
	parent := path[:len(path)-1]
	if len(parent) == 0 {
		return doc.insertLine(doc.root, path, raw, nil, false)
	}

	if scopes := doc.scopesAt(parent); len(scopes) > 1 {
		return nil, fmt.Errorf("toml insert %s: duplicate parent container", formatTOMLPath(parent))
	} else if len(scopes) == 1 {
		return doc.insertIntoScope(scopes[0], path[len(scopes[0].path):], raw, nil, false)
	}

	if scope, exemplar, ok := doc.findDottedScope(parent); ok {
		return doc.insertIntoScope(scope, path[len(scope.path):], raw, exemplar, false)
	}

	ancestor := doc.deepestExistingAncestor(parent)
	if len(ancestor) > 0 {
		if scopes := doc.scopesAt(ancestor); len(scopes) > 1 {
			return nil, fmt.Errorf("toml insert %s: duplicate ancestor container", formatTOMLPath(ancestor))
		} else if len(scopes) == 1 {
			scope := scopes[0]
			switch scope.kind {
			case unstable.InlineTable:
				return doc.insertInline(scope, path[len(scope.path):], raw, nil, true)
			case unstable.Table:
				if exemplar := lastTOMLDottedBinding(scope); exemplar != nil {
					return doc.insertLine(scope, path[len(scope.path):], raw, exemplar, false)
				}
				if exemplar := lastTOMLInlineBinding(scope); exemplar != nil {
					return doc.insertLine(scope, path[len(scope.path):], raw, exemplar, true)
				}
			}
		}
		if scope, exemplar, ok := doc.findDottedScope(ancestor); ok {
			return doc.insertIntoScope(scope, path[len(scope.path):], raw, exemplar, false)
		}
	}

	return doc.appendStandardTable(parent, path[len(path)-1], raw)
}

func (doc *tomlEditDocument) insertIntoScope(scope *tomlEditScope, relative []string, raw string, exemplar *tomlEditKeyValue, nested bool) ([]byte, error) {
	if scope.kind == unstable.InlineTable {
		return doc.insertInline(scope, relative, raw, exemplar, nested)
	}
	return doc.insertLine(scope, relative, raw, exemplar, nested)
}

func (doc *tomlEditDocument) insertLine(scope *tomlEditScope, relative []string, raw string, exemplar *tomlEditKeyValue, nested bool) ([]byte, error) {
	if scope.kind == unstable.InlineTable || len(relative) == 0 {
		return nil, fmt.Errorf("toml insert: invalid line scope")
	}
	format := exemplar
	var anchor *tomlEditKeyValue
	if scope.kind == unstable.Table {
		if exemplar != nil {
			anchor = exemplar
		} else if len(scope.bindings) > 0 {
			anchor = scope.bindings[len(scope.bindings)-1]
		}
		if format == nil {
			format = anchor
		}
	} else {
		if exemplar != nil {
			anchor = exemplar
		} else if len(scope.bindings) > 0 {
			anchor = scope.bindings[len(scope.bindings)-1]
		}
		if format == nil {
			format = anchor
		}
	}

	separator := " = "
	indent := ""
	if format != nil {
		separator = string(doc.data[format.keys[len(format.keys)-1].source.end:format.value.source.start])
		lineStart := tomlLineStart(doc.data, format.source.start)
		indent = string(doc.data[lineStart:format.source.start])
	}
	key := formatTOMLKeyPathLike(doc.data, relative, format)
	assignment := key + separator + raw
	if nested {
		assignment = formatTOMLNestedAssignment(relative, raw, separator, doc.data, format)
	}
	line := indent + assignment

	if anchor != nil {
		return insertTOMLLineAfter(doc.data, anchor.source.end, line), nil
	}
	if scope.kind == unstable.Table {
		return insertTOMLLineAfter(doc.data, scope.expr.source.end, line), nil
	}
	return insertTOMLLineAtRootStart(doc.data, line), nil
}

func (doc *tomlEditDocument) insertInline(scope *tomlEditScope, relative []string, raw string, exemplar *tomlEditKeyValue, nested bool) ([]byte, error) {
	if scope.kind != unstable.InlineTable || len(relative) == 0 {
		return nil, fmt.Errorf("toml insert: invalid inline scope")
	}
	format := exemplar
	if format == nil && len(scope.bindings) > 0 {
		format = scope.bindings[0]
	}
	separator := " = "
	if format != nil {
		separator = string(doc.data[format.keys[len(format.keys)-1].source.end:format.value.source.start])
	}
	assignment := formatTOMLKeyPathLike(doc.data, relative, format) + separator + raw
	if nested {
		assignment = formatTOMLNestedAssignment(relative, raw, separator, doc.data, format)
	}

	open := scope.value.source.start
	position := open + 1
	if len(scope.value.children) > 0 {
		for position < len(doc.data) && (doc.data[position] == ' ' || doc.data[position] == '\t') {
			position++
		}
		assignment += doc.inlineSeparator(scope.value)
	}
	return patchTOMLBytes(doc.data, position, position, []byte(assignment)), nil
}

func (doc *tomlEditDocument) inlineSeparator(value *tomlEditValue) string {
	for i := 0; i+1 < len(value.children); i++ {
		start := value.children[i].value.source.end
		end := value.children[i+1].source.start
		if bytes.IndexByte(doc.data[start:end], ',') >= 0 {
			return string(doc.data[start:end])
		}
	}
	return ", "
}

func (doc *tomlEditDocument) findDottedScope(container []string) (*tomlEditScope, *tomlEditKeyValue, bool) {
	var bestScope *tomlEditScope
	var bestBinding *tomlEditKeyValue
	for _, scope := range doc.scopes {
		if len(scope.path) >= len(container) || !tomlPathHasPrefix(container, scope.path) {
			continue
		}
		if bestScope != nil && len(scope.path) < len(bestScope.path) {
			continue
		}
		relative := container[len(scope.path):]
		for _, binding := range scope.bindings {
			keys := tomlEditKeyTexts(binding.keys)
			if len(keys) > len(relative) && tomlPathsEqual(keys[:len(relative)], relative) {
				if bestScope == nil || len(scope.path) > len(bestScope.path) {
					bestScope = scope
					bestBinding = binding
				} else if scope == bestScope {
					bestBinding = binding
				}
			}
		}
	}
	return bestScope, bestBinding, bestScope != nil
}

func (doc *tomlEditDocument) deepestExistingAncestor(parent []string) []string {
	for length := len(parent); length > 0; length-- {
		if value, found := doc.lookup(parent[:length]); found && isTOMLMap(value) {
			return append([]string(nil), parent[:length]...)
		}
	}
	return nil
}

func (doc *tomlEditDocument) appendStandardTable(parent []string, key, raw string) ([]byte, error) {
	if len(parent) == 0 {
		return nil, fmt.Errorf("toml insert: cannot append root table")
	}
	newline := detectTOMLNewline(doc.data)
	header := "[" + formatTOMLKeyPathLike(doc.data, parent, nil) + "]"
	body := formatTOMLKey(key) + " = " + raw
	block := header + newline + body
	finalNewline := len(doc.data) == 0 || hasTOMLFinalNewline(doc.data)
	if finalNewline {
		block += newline
	}
	if len(doc.data) > 0 && !hasTOMLFinalNewline(doc.data) {
		block = newline + block
	}
	return patchTOMLBytes(doc.data, len(doc.data), len(doc.data), []byte(block)), nil
}

func parseTOMLEditLiteral(raw string) (any, error) {
	const prefix = "value = "
	data := []byte(prefix + raw)
	doc, err := parseTOMLEditDocument(data)
	if err != nil {
		return nil, err
	}
	if len(doc.exprs) != 1 || len(doc.root.bindings) != 1 {
		return nil, fmt.Errorf("expected exactly one value literal")
	}
	binding := doc.root.bindings[0]
	if len(binding.keys) != 1 || binding.keys[0].text != "value" || binding.value.source.start != len(prefix) || binding.value.source.end != len(data) {
		return nil, fmt.Errorf("expected one literal without surrounding input")
	}
	value, found := doc.lookup([]string{"value"})
	if !found {
		return nil, fmt.Errorf("literal did not decode")
	}
	return value, nil
}

func validateTOMLEditCandidate(candidate []byte) ([]byte, error) {
	var semantic map[string]any
	if err := toml.Unmarshal(candidate, &semantic); err != nil {
		return nil, fmt.Errorf("invalid TOML edit candidate: %w", err)
	}
	// The unstable parser catches any discrepancy between the two frontends
	// before the candidate is allowed to escape this module.
	if _, err := parseTOMLEditDocument(candidate); err != nil {
		return nil, fmt.Errorf("invalid TOML edit candidate: %w", err)
	}
	return candidate, nil
}

func insertTOMLLineAfter(data []byte, anchorEnd int, line string) []byte {
	lineEnd := anchorEnd
	for lineEnd < len(data) && data[lineEnd] != '\n' {
		lineEnd++
	}
	if lineEnd < len(data) {
		newline := "\n"
		if lineEnd > 0 && data[lineEnd-1] == '\r' {
			newline = "\r\n"
		}
		insertAt := lineEnd + 1
		return patchTOMLBytes(data, insertAt, insertAt, []byte(line+newline))
	}
	newline := detectTOMLNewline(data)
	return patchTOMLBytes(data, len(data), len(data), []byte(newline+line))
}

func insertTOMLLineAtRootStart(data []byte, line string) []byte {
	if len(data) == 0 {
		return []byte(line + "\n")
	}
	return patchTOMLBytes(data, 0, 0, []byte(line+detectTOMLNewline(data)))
}

func deleteTOMLLine(data []byte, source tomlEditRange) []byte {
	start := tomlLineStart(data, source.start)
	end := source.end
	for end < len(data) && data[end] != '\n' {
		end++
	}
	if end < len(data) {
		return patchTOMLBytes(data, start, end+1, nil)
	}
	if start > 0 {
		start--
		if start > 0 && data[start-1] == '\r' {
			start--
		}
	}
	return patchTOMLBytes(data, start, end, nil)
}

func tomlLineStart(data []byte, offset int) int {
	if index := bytes.LastIndexByte(data[:offset], '\n'); index >= 0 {
		return index + 1
	}
	return 0
}

func detectTOMLNewline(data []byte) string {
	if index := bytes.IndexByte(data, '\n'); index > 0 && data[index-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

func hasTOMLFinalNewline(data []byte) bool {
	return len(data) > 0 && data[len(data)-1] == '\n'
}

func patchTOMLBytes(data []byte, start, end int, replacement []byte) []byte {
	result := make([]byte, 0, len(data)-(end-start)+len(replacement))
	result = append(result, data[:start]...)
	result = append(result, replacement...)
	result = append(result, data[end:]...)
	return result
}

func formatTOMLNestedAssignment(path []string, raw, separator string, data []byte, exemplar *tomlEditKeyValue) string {
	if len(path) == 1 {
		return formatTOMLKeyPathLike(data, path, exemplar) + separator + raw
	}
	key := formatTOMLKey(path[0])
	if exemplar != nil && len(exemplar.keys) > 0 && exemplar.keys[0].text == path[0] {
		key = string(data[exemplar.keys[0].source.start:exemplar.keys[0].source.end])
	}
	return key + separator + "{ " + formatTOMLNestedAssignment(path[1:], raw, separator, nil, nil) + " }"
}

func formatTOMLKeyPathLike(data []byte, path []string, exemplar *tomlEditKeyValue) string {
	var builder strings.Builder
	for i, part := range path {
		if i > 0 {
			separator := "."
			if exemplar != nil && i < len(exemplar.keys) {
				separator = string(data[exemplar.keys[i-1].source.end:exemplar.keys[i].source.start])
			}
			builder.WriteString(separator)
		}
		key := ""
		if exemplar != nil && i < len(exemplar.keys) && exemplar.keys[i].text == part {
			key = string(data[exemplar.keys[i].source.start:exemplar.keys[i].source.end])
		}
		if key == "" {
			key = formatTOMLKey(part)
		}
		builder.WriteString(key)
	}
	return builder.String()
}

func formatTOMLKey(key string) string {
	if key != "" && utf8.ValidString(key) {
		bare := true
		for i := 0; i < len(key); i++ {
			c := key[i]
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
				bare = false
				break
			}
		}
		if bare {
			return key
		}
	}
	encoded, _ := json.Marshal(key)
	return string(encoded)
}

func formatTOMLPath(path []string) string {
	parts := make([]string, len(path))
	for i, part := range path {
		parts[i] = formatTOMLKey(part)
	}
	return strings.Join(parts, ".")
}

func tomlEditKeyTexts(keys []tomlEditKey) []string {
	result := make([]string, len(keys))
	for i, key := range keys {
		result[i] = key.text
	}
	return result
}

func appendTOMLPath(base []string, parts ...string) []string {
	result := make([]string, 0, len(base)+len(parts))
	result = append(result, base...)
	result = append(result, parts...)
	return result
}

func tomlPathsEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func tomlPathHasPrefix(path, prefix []string) bool {
	return len(prefix) <= len(path) && tomlPathsEqual(path[:len(prefix)], prefix)
}

func tomlPathInsideArray(arrayPaths [][]string, path []string) bool {
	for _, arrayPath := range arrayPaths {
		if tomlPathHasPrefix(path, arrayPath) {
			return true
		}
	}
	return false
}

func lastTOMLDottedBinding(scope *tomlEditScope) *tomlEditKeyValue {
	for i := len(scope.bindings) - 1; i >= 0; i-- {
		if len(scope.bindings[i].keys) > 1 {
			return scope.bindings[i]
		}
	}
	return nil
}

func lastTOMLInlineBinding(scope *tomlEditScope) *tomlEditKeyValue {
	for i := len(scope.bindings) - 1; i >= 0; i-- {
		if len(scope.bindings[i].keys) == 1 && scope.bindings[i].value.kind == unstable.InlineTable {
			return scope.bindings[i]
		}
	}
	return nil
}

func isTOMLScalarKind(kind unstable.Kind) bool {
	return kind != unstable.InlineTable && kind != unstable.Array
}

func isTOMLMap(value any) bool {
	_, ok := value.(map[string]any)
	return ok
}

func isTOMLContainerValue(value any) bool {
	if isTOMLMap(value) {
		return true
	}
	_, ok := value.([]map[string]any)
	if ok {
		return true
	}
	_, ok = value.([]any)
	return ok
}
