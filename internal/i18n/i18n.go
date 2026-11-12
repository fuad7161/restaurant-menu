// Package i18n holds the guest-facing texts.
// Guest screens are in Bangla and English. Staff screens are English only.
package i18n

import "net/http"

func Lang(r *http.Request) string {
	if c, err := r.Cookie("lang"); err == nil && c.Value == "en" {
		return "en"
	}
	return "bn"
}

func T(lang, key string) string {
	if s, ok := texts[key]; ok {
		if lang == "bn" {
			return s[1]
		}
		return s[0]
	}
	return key
}

// key: {English, Bangla}
var texts = map[string][2]string{
	"table":            {"Table", "টেবিল"},
	"group":            {"Group", "গ্রুপ"},
	"closed_now":       {"The restaurant is not taking orders right now.", "রেস্টুরেন্ট এখন অর্ডার নিচ্ছে না।"},
	"min":              {"min", "মিনিট"},
	"sold_out":         {"Sold out", "শেষ হয়ে গেছে"},
	"add":              {"Add", "যোগ করুন"},
	"added":            {"Added", "যোগ হয়েছে"},
	"yes_add":          {"Yes, add", "হ্যাঁ, যোগ করুন"},
	"no":               {"No", "না"},
	"wants_join":       {"wants to join your order", "আপনাদের অর্ডারে যোগ দিতে চান"},
	"allow":            {"Allow", "অনুমতি দিন"},
	"decline":          {"Decline", "না"},
	"menu":             {"Menu", "মেনু"},
	"cart":             {"Cart", "কার্ট"},
	"orders":           {"Orders", "অর্ডার"},
	"review_order":     {"Review your order", "অর্ডার মিলিয়ে নিন"},
	"cart_empty":       {"The cart is empty.", "কার্ট খালি।"},
	"note_ph":          {"Note: less spicy, no onion…", "নোট: ঝাল কম, পেঁয়াজ ছাড়া…"},
	"subtotal":         {"Subtotal", "উপমোট"},
	"place_order":      {"Place order", "অর্ডার দিন"},
	"paid":             {"Paid", "পরিশোধিত"},
	"thanks":           {"Thank you for dining with us.", "আমাদের সাথে খাওয়ার জন্য ধন্যবাদ।"},
	"no_orders":        {"No orders yet. Pick something from the menu.", "এখনো কোনো অর্ডার নেই। মেনু থেকে বেছে নিন।"},
	"order":            {"Order", "অর্ডার"},
	"addon":            {"add-on", "অতিরিক্ত"},
	"st_placed":        {"Received", "গৃহীত"},
	"st_cooking":       {"Cooking", "রান্না হচ্ছে"},
	"st_ready":         {"Ready", "প্রস্তুত"},
	"st_served":        {"Served", "পরিবেশিত"},
	"st_rejected":      {"Rejected", "বাতিল করা হয়েছে"},
	"st_cancelled":     {"Cancelled", "বাতিল"},
	"wait_accept":      {"Waiting for the restaurant to accept.", "রেস্টুরেন্টের গ্রহণের অপেক্ষায়।"},
	"ready_in":         {"Ready in about", "প্রস্তুত হবে আনুমানিক"},
	"any_moment":       {"Ready any moment now", "যেকোনো মুহূর্তে প্রস্তুত হবে"},
	"change_requested": {"Change requested, waiting for staff…", "পরিবর্তনের অনুরোধ পাঠানো হয়েছে…"},
	"change_declined":  {"Staff could not change this item.", "এই আইটেম পরিবর্তন করা যায়নি।"},
	"change_approved":  {"Changed as you asked.", "আপনার অনুরোধ অনুযায়ী পরিবর্তন হয়েছে।"},
	"remove_one":       {"−1", "−১"},
	"cancel_item":      {"Cancel", "বাতিল"},
	"ask_remove_one":   {"Ask −1", "−১ অনুরোধ"},
	"ask_cancel":       {"Ask to cancel", "বাতিলের অনুরোধ"},
	"confirm_cancel":   {"Cancel this item?", "এই আইটেম বাতিল করবেন?"},
	"bill":             {"Bill", "বিল"},
	"service_charge":   {"Service charge", "সার্ভিস চার্জ"},
	"vat":              {"VAT", "ভ্যাট"},
	"total":            {"Total", "মোট"},
	"request_bill":     {"Request bill", "বিল চান"},
	"bill_requested":   {"Bill requested. Pay the waiter or at the counter.", "বিল চাওয়া হয়েছে। ওয়েটার বা কাউন্টারে পরিশোধ করুন।"},
	"keep_ordering":    {"Order more instead", "আরও অর্ডার করব"},
	"call_waiter":      {"Call waiter", "ওয়েটার ডাকুন"},
	"water":            {"Water", "পানি"},
	"tissue":           {"Tissue", "টিস্যু"},
	"waiter_coming":    {"The staff have been notified.", "স্টাফকে জানানো হয়েছে।"},
	"rate_visit":       {"How was it?", "কেমন লাগল?"},
	"overall":          {"Overall", "সামগ্রিক"},
	"comment_ph":       {"Tell us more (optional)", "আরও কিছু বলুন (ঐচ্ছিক)"},
	"send_review":      {"Send review", "রিভিউ পাঠান"},
	"thanks_review":    {"Thanks for your review!", "রিভিউয়ের জন্য ধন্যবাদ!"},
	"new_order":        {"Start a new order", "নতুন অর্ডার শুরু করুন"},
	"welcome":          {"Welcome! Order from your phone.", "স্বাগতম! ফোন থেকেই অর্ডার করুন।"},
	"your_name":        {"Your name", "আপনার নাম"},
	"start_order":      {"Start ordering", "অর্ডার শুরু করুন"},
	"table_has_order":  {"This table already has an order. What would you like to do?", "এই টেবিলে আগে থেকেই অর্ডার চলছে। আপনি কী করতে চান?"},
	"join_group":       {"Join the order of", "যোগ দিন — অর্ডার:"},
	"start_separate":   {"Start a separate order", "আলাদা অর্ডার শুরু করুন"},
	"continue":         {"Continue", "এগিয়ে যান"},
	"waiting_approval": {"Waiting for someone at the table to allow you…", "টেবিলের কারও অনুমতির অপেক্ষায়…"},
	"join_declined":    {"Your request was not accepted.", "আপনার অনুরোধ গ্রহণ করা হয়নি।"},
	"not_ordering":     {"Ordering is closed for this bill.", "এই বিলে আর অর্ডার নেওয়া হচ্ছে না।"},
	"dup_warn":         {"%s already added %d × %s. Add another?", "%s আগেই %d × %s যোগ করেছেন। আরেকটি যোগ করবেন?"},
	"err_soldout":      {"%s is sold out. Please remove it.", "%s শেষ হয়ে গেছে। অনুগ্রহ করে বাদ দিন।"},
	"err_wait":         {"Please wait a few seconds before ordering again.", "আবার অর্ডার দেওয়ার আগে কয়েক সেকেন্ড অপেক্ষা করুন।"},
	"added_by":         {"added by", "যোগ করেছেন"},
	"each":             {"each", "প্রতিটি"},
}
