package server

import "testing"

// The templates parse, and every template name the handlers render exists.
func TestTemplatesParse(t *testing.T) {
	tpl, err := newTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"guest_menu", "g_sheet", "g_add", "guest_cart", "g_cartlist", "g_live", "guest_receipt",
		"toast", "g_bar", "kitchen", "k_board", "stock_row", "counter", "session", "staff_tables", "owner_item", "owner_settings"} {
		if tpl.Lookup(name) == nil {
			t.Errorf("template %q is missing", name)
		}
	}
}
