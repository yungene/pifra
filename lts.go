package pifra

import (
	"bytes"
	"container/list"
	"encoding/gob"
	"fmt"
	stdlog "log"
	"sort"
	"strconv"
	"text/template"
)

type Lts struct {
	States      map[int]Configuration
	Transitions []Transition

	RegSizeReached map[int]bool

	StatesExplored  int
	StatesGenerated int

	FreeNamesMap map[string]string
}

type Transition struct {
	Source      int
	Destination int
	Label       Label
}

type VertexTemplate struct {
	State  string
	Config string
	Layout string
}

type EdgeTemplate struct {
	Source      string
	Destination string
	Label       string
}

var a4GVLayout = []byte(`
    size="8.3,11.7!";
    ratio="fill";
    margin=0;
    rankdir = TB;
`)

var gvLayout string

func explore(root Configuration) Lts {
	// Visited states.
	visited := make(map[string]int)
	// Encountered transitions.
	trnsSeen := make(map[Transition]bool)
	// Track which states have reached the register size.
	regSizeReached := make(map[int]bool)
	// LTS states.
	states := make(map[int]Configuration)
	// LTS transitions.
	var trns []Transition
	// State ID.
	var stateId int

	applyStructrualCongruence(root)
	rootKey := getConfigurationKey(root)
	visited[rootKey] = stateId
	states[stateId] = root
	stateId++

	queue := list.New()
	queue.PushBack(root)
	dequeue := func() Configuration {
		c := queue.Front()
		queue.Remove(c)
		return c.Value.(Configuration)
	}

	var statesExplored int
	var statesGenerated int

	// BFS traversal state exploration.
	for queue.Len() > 0 && statesExplored < maxStatesExplored {
		state := dequeue()

		srcId := visited[getConfigurationKey(state)]

		if len(state.Registers.Registers) > registerSize {
			regSizeReached[srcId] = true
		} else {
			confs := trans(state)
			for _, conf := range confs {
				statesGenerated++
				applyStructrualCongruence(conf)
				dstKey := getConfigurationKey(conf)
				if _, ok := visited[dstKey]; !ok {
					visited[dstKey] = stateId
					states[stateId] = conf
					stateId++
					queue.PushBack(conf)
				}
				trn := Transition{
					Source:      srcId,
					Destination: visited[dstKey],
					Label:       conf.Label,
				}
				if !trnsSeen[trn] {
					trnsSeen[trn] = true
					trns = append(trns, trn)
				}
			}
		}

		statesExplored++
	}

	return Lts{
		States:          states,
		Transitions:     trns,
		RegSizeReached:  regSizeReached,
		StatesExplored:  statesExplored,
		StatesGenerated: statesGenerated,
	}
}

func init() {
	RegisterGobs()
}

// RegisterGobs registers concrete types implementing the Element interface for
// encoding as binary gobs.
func RegisterGobs() {
	gob.Register(&ElemNil{})
	gob.Register(&ElemOutput{})
	gob.Register(&ElemInput{})
	gob.Register(&ElemEquality{})
	gob.Register(&ElemRestriction{})
	gob.Register(&ElemSum{})
	gob.Register(&ElemParallel{})
	gob.Register(&ElemProcess{})
	gob.Register(&ElemRoot{})
}

func generateGobFile(lts Lts) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(lts)
	if err != nil {
		stdlog.Fatal(err)
	}
	return buf.Bytes()
}

func GenerateGraphVizFile(lts Lts, outputStateNo bool) []byte {
	vertices := lts.States
	edges := lts.Transitions

	var buffer bytes.Buffer

	gvl := ""
	if gvLayout != "" {
		gvl = "\n    " + gvLayout + "\n"
	}
	buffer.WriteString("digraph {" + gvl + "\n")

	var ids []int
	for id := range vertices {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for _, id := range ids {
		conf := vertices[id]

		var config string
		if outputStateNo {
			config = "s" + strconv.Itoa(id)
		} else {
			config = PrettyPrintRegister(conf.Registers) + " ⊢\n" + PrettyPrintAst(conf.Process)
		}

		var layout string
		if id == 0 {
			layout = layout + "peripheries=2,"
		}
		if lts.RegSizeReached[id] {
			layout = layout + "peripheries=3,"
		}

		vertex := VertexTemplate{
			State:  "s" + strconv.Itoa(id),
			Config: config,
			Layout: layout,
		}
		var tmpl *template.Template
		tmpl, _ = template.New("todos").Parse("    {{.State}} [{{.Layout}}label=\"{{.Config}}\"]\n")
		tmpl.Execute(&buffer, vertex)
	}

	buffer.WriteString("\n")

	for _, edge := range edges {
		edg := EdgeTemplate{
			Source:      "s" + strconv.Itoa(edge.Source),
			Destination: "s" + strconv.Itoa(edge.Destination),
			Label:       PrettyPrintGraphLabel(edge.Label),
		}
		tmpl, _ := template.New("todos").Parse("    {{.Source}} -> {{.Destination}} [label=\"{{ .Label}}\"]\n")
		tmpl.Execute(&buffer, edg)
	}

	buffer.WriteString("}\n")

	var output bytes.Buffer
	buffer.WriteTo(&output)
	return output.Bytes()
}

func (l Label) PrettyPrintGraph() string {
	return PrettyPrintGraphLabel(l)
}

func PrettyPrintGraphLabel(label Label) string {
	if label.Symbol.Type == SymbolTypTau {
		return "τ"
	}
	return PrettyPrintGraphSymbol(label.Symbol) + PrettyPrintGraphSymbol(label.Symbol2)
}

func PrettyPrintGraphSymbol(symbol Symbol) string {
	s := symbol.Value
	switch symbol.Type {
	case SymbolTypInput:
		return strconv.Itoa(s) + " "
	case SymbolTypOutput:
		return strconv.Itoa(s) + "' "
	case SymbolTypFreshInput:
		return strconv.Itoa(s) + "●"
	case SymbolTypFreshOutput:
		return strconv.Itoa(s) + "⊛"
	case SymbolTypTau:
		return "τ"
	case SymbolTypKnown:
		return strconv.Itoa(s)
	}
	return ""
}

func generateGraphVizTexFile(lts Lts, outputStateNo bool) []byte {
	vertices := lts.States
	edges := lts.Transitions

	var buffer bytes.Buffer

	gvl := ""
	if gvLayout != "" {
		gvl = "\n    " + gvLayout + "\n"
	}
	buffer.WriteString("digraph {" + gvl + "\n")

	buffer.WriteString(`    d2toptions="--format tikz --crop --autosize --nominsize";`)
	buffer.WriteString("\n")
	buffer.WriteString(`    d2tdocpreamble="\usepackage{amssymb}";`)
	buffer.WriteString("\n\n")

	var ids []int
	for id := range vertices {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for _, id := range ids {
		conf := vertices[id]

		var config string
		if outputStateNo {
			config = "s_{" + strconv.Itoa(id) + "}"
		} else {
			config = `\begin{matrix} ` +
				PrettyPrintTexRegister(conf.Registers) +
				` \vdash \\ ` +
				PrettyPrintTexAst(conf.Process) +
				` \end{matrix}`
		}

		var layout string
		if id == 0 {
			layout = layout + `style="double",`
		}
		if lts.RegSizeReached[id] {
			layout = layout + `style="thick",`
		}

		vertex := VertexTemplate{
			State:  "s" + strconv.Itoa(id),
			Config: config,
			Layout: layout,
		}
		var tmpl *template.Template
		tmpl, _ = template.New("todos").Parse("    {{.State}} [{{.Layout}}texlbl=\"${{.Config}}$\"]\n")
		tmpl.Execute(&buffer, vertex)
	}

	buffer.WriteString("\n")

	for _, edge := range edges {
		edg := EdgeTemplate{
			Source:      "s" + strconv.Itoa(edge.Source),
			Destination: "s" + strconv.Itoa(edge.Destination),
			Label:       PrettyPrintTexGraphLabel(edge.Label),
		}
		tmpl, _ := template.New("todos").Parse(
			"    {{.Source}} -> {{.Destination}} [label=\"\",texlbl=\"${{.Label}}$\"]\n")
		tmpl.Execute(&buffer, edg)
	}

	buffer.WriteString("}\n")

	var output bytes.Buffer
	buffer.WriteTo(&output)
	return output.Bytes()
}

func PrettyPrintTexRegister(register Registers) string {
	str := `\{`
	labels := register.Labels()
	reg := register.Registers

	for i, label := range labels {
		if i == len(labels)-1 {
			str = str + "(" + strconv.Itoa(label) + "," + GetTexName(reg[label]) + ")"
		} else {
			str = str + "(" + strconv.Itoa(label) + "," + GetTexName(reg[label]) + "),"
		}
	}
	return str + `\}`
}

func GetTexName(name string) string {
	if string(name[0]) == "#" {
		return "a" + "_{" + name[1:] + "}"
	}
	if string(name[0]) == "&" {
		return "x" + "_{" + name[1:] + "}"
	}
	if string(name[0]) == "_" {
		return name[1:]
	}
	return name
}

// PrettyPrintAst returns a string containing the pi-calculus syntax of the AST.
func PrettyPrintTexAst(elem Element) string {
	return PrettyPrintTexAstAcc(elem, "")
}

func PrettyPrintTexAstAcc(elem Element, str string) string {
	elemTyp := elem.Type()
	switch elemTyp {
	case ElemTypNil:
		str += "0"
	case ElemTypOutput:
		outElem := elem.(*ElemOutput)
		str += fmt.Sprintf(`\bar{%s} \langle %s \rangle . `,
			GetTexName(outElem.Channel.Name), GetTexName(outElem.Output.Name))
		return PrettyPrintTexAstAcc(outElem.Next, str)
	case ElemTypInput:
		inpElem := elem.(*ElemInput)
		str += fmt.Sprintf(`%s ( %s ) . `,
			GetTexName(inpElem.Channel.Name), GetTexName(inpElem.Input.Name))
		return PrettyPrintTexAstAcc(inpElem.Next, str)
	case ElemTypMatch:
		matchElem := elem.(*ElemEquality)
		if matchElem.Inequality {
			str += fmt.Sprintf(`\lbrack %s \neq %s \rbrack . `,
				GetTexName(matchElem.NameL.Name), GetTexName(matchElem.NameR.Name))
		} else {
			str += fmt.Sprintf(`\lbrack %s = %s \rbrack . `,
				GetTexName(matchElem.NameL.Name), GetTexName(matchElem.NameR.Name))
		}
		return PrettyPrintTexAstAcc(matchElem.Next, str)
	case ElemTypRestriction:
		resElem := elem.(*ElemRestriction)
		str += fmt.Sprintf(`\nu %s . `,
			GetTexName(resElem.Restrict.Name))
		return PrettyPrintTexAstAcc(resElem.Next, str)
	case ElemTypSum:
		sumElem := elem.(*ElemSum)
		left := PrettyPrintTexAstAcc(sumElem.ProcessL, "")
		right := PrettyPrintTexAstAcc(sumElem.ProcessR, "")
		str += fmt.Sprintf(`( %s + %s )`, left, right)
	case ElemTypParallel:
		parElem := elem.(*ElemParallel)
		left := PrettyPrintTexAstAcc(parElem.ProcessL, "")
		right := PrettyPrintTexAstAcc(parElem.ProcessR, "")
		str += fmt.Sprintf(`( %s \mid %s )`, left, right)
	case ElemTypProcess:
		pcsElem := elem.(*ElemProcess)
		if len(pcsElem.Parameters) == 0 {
			str = str + pcsElem.Name
		} else {
			params := "("
			for i, param := range pcsElem.Parameters {
				if i == len(pcsElem.Parameters)-1 {
					params = params + GetTexName(param.Name) + ")"
				} else {
					params = params + GetTexName(param.Name) + ", "
				}
			}
			str = str + pcsElem.Name + params
		}
	case ElemTypRoot:
		rootElem := elem.(*ElemRoot)
		return PrettyPrintTexAstAcc(rootElem.Next, str)
	}
	return str
}

func PrettyPrintTexGraphLabel(label Label) string {
	if label.Symbol.Type == SymbolTypTau {
		return `\tau`
	}
	return PrettyPrintTexGraphSymbol(label.Symbol) + ` \, ` + PrettyPrintTexGraphSymbol(label.Symbol2)
}

func PrettyPrintTexGraphSymbol(symbol Symbol) string {
	s := symbol.Value
	switch symbol.Type {
	case SymbolTypInput:
		return strconv.Itoa(s)
	case SymbolTypOutput:
		return `\bar{` + strconv.Itoa(s) + `}`
	case SymbolTypFreshInput:
		return strconv.Itoa(s) + `^{\bullet}`
	case SymbolTypFreshOutput:
		return strconv.Itoa(s) + `^{\circledast}`
	case SymbolTypTau:
		return `\tau`
	case SymbolTypKnown:
		return strconv.Itoa(s)
	}
	return ""
}

func generatePrettyLts(lts Lts) []byte {
	vertices := lts.States
	edges := lts.Transitions

	// When there is no root state.
	if _, ok := vertices[0]; !ok {
		return []byte{}
	}
	var buffer bytes.Buffer

	root := vertices[0]

	rootR := ""
	if lts.RegSizeReached[0] {
		rootR = "+"
	}

	rootString := "s0" + rootR + " = " +
		PrettyPrintRegister(root.Registers) + " |- " + PrettyPrintAst(root.Process)
	buffer.WriteString(rootString)

	// Prevent extraneous new line if there are no edges.
	if len(edges) != 0 {
		buffer.WriteString("\n")
	}

	for i, edge := range edges {
		vertex := vertices[edge.Destination]
		srcR := ""
		if lts.RegSizeReached[edge.Source] {
			srcR = "+"
		}
		dstR := ""
		if lts.RegSizeReached[edge.Destination] {
			dstR = "+"
		}
		transString := "s" + strconv.Itoa(edge.Source) + srcR + "  " +
			PrettyPrintLabel(edge.Label) + "  s" + strconv.Itoa(edge.Destination) + dstR + " = " +
			PrettyPrintRegister(vertex.Registers) + " |- " + PrettyPrintAst(vertex.Process)
		buffer.WriteString(transString)

		// Prevent extraneous new line at last edge.
		if i != len(edges)-1 {
			buffer.WriteString("\n")
		}
	}

	var output bytes.Buffer
	buffer.WriteTo(&output)
	return output.Bytes()
}

// PrettyPrintConfiguration returns a pretty printed string of the configuration.
func PrettyPrintConfiguration(conf Configuration) string {
	return PrettyPrintLabel(conf.Label) + " -> " + PrettyPrintRegister(conf.Registers) + " ¦- " +
		PrettyPrintAst(conf.Process)

}

func PrettyPrintRegister(register Registers) string {
	str := "{"
	labels := register.Labels()
	reg := register.Registers

	for i, label := range labels {
		if i == len(labels)-1 {
			str = str + "(" + strconv.Itoa(label) + "," + reg[label] + ")"
		} else {
			str = str + "(" + strconv.Itoa(label) + "," + reg[label] + "),"
		}
	}
	return str + "}"
}

func PrettyPrintLabel(label Label) string {
	if label.Symbol.Type == SymbolTypTau {
		return "t   "
	}
	return PrettyPrintSymbol(label.Symbol) + PrettyPrintSymbol(label.Symbol2)
}

func PrettyPrintSymbol(symbol Symbol) string {
	s := symbol.Value
	switch symbol.Type {
	case SymbolTypInput:
		return strconv.Itoa(s) + " "
	case SymbolTypOutput:
		return strconv.Itoa(s) + "'"
	case SymbolTypFreshInput:
		return strconv.Itoa(s) + "*"
	case SymbolTypFreshOutput:
		return strconv.Itoa(s) + "^"
	case SymbolTypTau:
		return "t   "
	case SymbolTypKnown:
		return strconv.Itoa(s) + " "
	}
	return ""
}
