package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pieropalacioscode/ladmincabo/internal/importer"
)

type attrStat struct {
	n, empty int
	values   map[string]int
}

// attrStats resume los atributos de un archivo para decidir cuáles sirven como filtro.
type attrStats struct{ by map[string]*attrStat }

func newAttrStats() *attrStats { return &attrStats{by: map[string]*attrStat{}} }

func (s *attrStats) addValue(name, value string) {
	name = strings.TrimSpace(name)
	if name == "" || skipAttributeNames[strings.ToLower(name)] {
		return
	}
	st := s.by[name]
	if st == nil {
		st = &attrStat{values: map[string]int{}}
		s.by[name] = st
	}
	v := normalizeMaybeEmpty(value)
	if v == "" {
		st.empty++
		return
	}
	st.n++
	st.values[v]++
}

func (s *attrStats) add(blocks [16]importer.Atributo) {
	for _, a := range blocks {
		s.addValue(a.Nombre, a.Valor)
	}
}

func (s *attrStats) variants(name string) [][]string {
	st := s.by[name]
	if st == nil {
		return nil
	}
	return caseVariants(st.values)
}

func topValues(m map[string]int, n int) []string {
	type kv struct {
		v string
		n int
	}
	list := make([]kv, 0, len(m))
	for v, c := range m {
		list = append(list, kv{v, c})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].v < list[j].v
	})
	out := []string{}
	for i, e := range list {
		if i >= n {
			break
		}
		v := e.v
		if r := []rune(v); len(r) > 22 {
			v = string(r[:22]) + "…"
		}
		out = append(out, fmt.Sprintf("%s=%d", v, e.n))
	}
	return out
}

func (s *attrStats) report() {
	if len(s.by) == 0 {
		return
	}
	type kv struct {
		name string
		st   *attrStat
	}
	list := make([]kv, 0, len(s.by))
	for n, st := range s.by {
		list = append(list, kv{n, st})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].st.n != list[j].st.n {
			return list[i].st.n > list[j].st.n
		}
		return list[i].name < list[j].name
	})
	fmt.Println("Atributos detectados (productos con valor · valores distintos · vacíos o «-»):")
	for i, e := range list {
		if i >= 20 {
			fmt.Printf("   … y %d más\n", len(list)-20)
			break
		}
		fmt.Printf("   - %-26s %4d · %3d valores · %3d vacíos   ej.: %s\n", e.name, e.st.n, len(e.st.values), e.st.empty, strings.Join(topValues(e.st.values, 3), ", "))
		for j, g := range s.variants(e.name) {
			if j >= 2 {
				break
			}
			fmt.Printf("       ⚠️  solo difieren en mayúsculas o tildes: %s\n", strings.Join(g, " / "))
		}
	}
}
