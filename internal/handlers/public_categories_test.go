package handlers

import "testing"

func TestCategoryTreeYSubarbol(t *testing.T) {
	p := uint(1)
	cats := []categoryRow{
		{ID: 1, Name: "Grupo", Slug: "grupo"},
		{ID: 2, Name: "Hoja A", Slug: "hoja-a", ParentID: &p},
		{ID: 3, Name: "Hoja B", Slug: "hoja-b", ParentID: &p},
		{ID: 4, Name: "Vacia", Slug: "vacia", ParentID: &p},
	}
	got := categoryTree(cats, map[uint]int64{2: 3, 3: 2})
	if len(got) != 3 {
		t.Fatalf("categorías = %d, quería 3 (la vacía no aparece)", len(got))
	}
	counts := map[string]int64{}
	for _, c := range got {
		counts[c.Slug] = c.ProductCount
	}
	if counts["grupo"] != 5 || counts["hoja-a"] != 3 || counts["hoja-b"] != 2 {
		t.Errorf("conteos = %v", counts)
	}
	for _, c := range got {
		if c.Slug == "grupo" && c.ParentSlug != nil {
			t.Error("el grupo raíz no debe tener parent_slug")
		}
		if c.Slug == "hoja-a" && (c.ParentSlug == nil || *c.ParentSlug != "grupo") {
			t.Error("hoja-a debe apuntar a grupo")
		}
	}
	if n := len(subtreeFrom(cats, 1)); n != 4 {
		t.Errorf("subárbol del grupo = %d ids, quería 4", n)
	}
	if ids := subtreeFrom(cats, 2); len(ids) != 1 || ids[0] != 2 {
		t.Errorf("subárbol de una hoja = %v, quería [2]", ids)
	}
}
