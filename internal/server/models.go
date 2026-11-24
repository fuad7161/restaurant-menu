package server

import (
	"fmt"
	"strconv"
)

type Restaurant struct {
	ID            int64
	Name          string
	VAT, Service  float64
	IsOpen        bool
	RequireAccept bool
	// profile, edited by the owner on /owner/profile
	Logo, Color, Tagline, Address, Phone string
	Networks                             string // order_networks: IP ranges guests may order from, one per line; empty = anywhere
	// opening hours shown to guests ("HH:MM", empty = not shown), and the kitchen ticket colour limits
	OpenTime, CloseTime, LastOrder string
	WarnMinutes, LateMinutes       int
	OrdersInHours                  bool   // guests can order only between OpenTime and LastOrder (or CloseTime)
	ReviewURL                      string // Google review link offered after a 4 or 5 star review
	BIN                            string // VAT registration number printed on bills and receipts
	VerifyEmail                    string // global | required | off: must the owner confirm their email (set by the platform admin)
	OwnerVerified                  bool   // an owner of this restaurant has confirmed their email
	NotReady                       bool   // guests: closed because the owner hasn't confirmed their email yet
	// branches (docs/plans/multi-branch.md): this restaurant is one branch of a business
	BusinessID             int64
	BusinessName           string
	BranchName, BranchCode string // "Gulshan", "GUL"; empty for a one-branch business
	Status                 string // active | closed | archived
}

// Closed reports whether the owner has closed this branch for now: guests can't order, staff can
// still log in.
func (r Restaurant) Closed() bool { return r.Status == "closed" }

// InvoiceLabel is how an invoice number is printed: "GUL-0042" when the branch has a code, or "42".
func (r Restaurant) InvoiceLabel(n int64) string {
	if r.BranchCode == "" {
		return strconv.FormatInt(n, 10)
	}
	return fmt.Sprintf("%s-%04d", r.BranchCode, n)
}

// restCols reads a whole restaurant (alias r) into the fields returned by dest.
// The name is the business's, and the logo, colour and tagline are the branch's own when it
// overrides them, else the business's.
const restCols = `r.id,(SELECT b.name FROM businesses b WHERE b.id=r.business_id),r.vat_percent,r.service_percent,r.is_open,r.require_accept,
	COALESCE(NULLIF(r.logo,''),(SELECT b.logo FROM businesses b WHERE b.id=r.business_id),''),
	COALESCE(NULLIF(r.brand_color,''),(SELECT b.brand_color FROM businesses b WHERE b.id=r.business_id),'#8f1d21'),
	COALESCE(NULLIF(r.tagline,''),(SELECT b.tagline FROM businesses b WHERE b.id=r.business_id),''),
	r.address,r.phone,r.order_networks,` + restExtraCols

func (r *Restaurant) dest() []any {
	return append([]any{&r.ID, &r.Name, &r.VAT, &r.Service, &r.IsOpen, &r.RequireAccept,
		&r.Logo, &r.Color, &r.Tagline, &r.Address, &r.Phone, &r.Networks}, r.extraDest()...)
}

// Branch is one entry of an owner's branch switcher.
type Branch struct {
	ID          int64
	Label, Code string // Label: the branch name, or the restaurant name when it has none
	Status      string
}

// DisplayName is what guests and bills show: "Kacchi Ghar · Gulshan", or just the business name
// when the branch has no name of its own.
func (r Restaurant) DisplayName() string {
	if r.BranchName == "" {
		return r.Name
	}
	return r.BusinessName + " · " + r.BranchName
}

// restExtraCols are the restaurant columns read into the fields after Networks (see extraDest).
// The owner belongs to the whole business, so a branch without an owner row of its own still
// finds them through business_id.
const restExtraCols = `r.open_time,r.close_time,r.last_order,r.warn_minutes,r.late_minutes,r.orders_in_hours,r.review_url,r.bin,r.verify_email,
	EXISTS(SELECT 1 FROM staff_users o JOIN restaurants ob ON ob.id=o.restaurant_id
		WHERE ob.business_id=r.business_id AND o.role='owner' AND o.email_verified_at>0),
	r.business_id,(SELECT b.name FROM businesses b WHERE b.id=r.business_id),r.branch_name,r.branch_code,r.status`

func (r *Restaurant) extraDest() []any {
	return []any{&r.OpenTime, &r.CloseTime, &r.LastOrder, &r.WarnMinutes, &r.LateMinutes, &r.OrdersInHours, &r.ReviewURL, &r.BIN, &r.VerifyEmail, &r.OwnerVerified,
		&r.BusinessID, &r.BusinessName, &r.BranchName, &r.BranchCode, &r.Status}
}

type Table struct {
	ID     int64
	Label  string
	Token  string
	Active bool
}

type Session struct {
	ID       int64
	Label    string
	Status   string
	OpenedAt int64
	ClosedAt int64
}

type Guest struct {
	ID      int64
	Name    string
	Status  string
	IsStaff bool
}

type Opt struct {
	ID      int64
	Name    string
	Extra   int64
	GroupID int64 // 0 = a free extra, not in a group
}

// OptGroup is a set of options with a rule, such as "Size: choose 1 (required)".
// ID 0 is the built-in group of free extras (options without a group).
type OptGroup struct {
	ID       int64
	Name     string
	Min, Max int // Min >= 1 means required; Max 0 means no limit, 1 means one choice only
	Options  []Opt
}

type Item struct {
	ID, CatID      int64
	NameBn, NameEn string
	DescBn, DescEn string
	Price          int64
	Photo          string
	Prep           int
	Available      bool
	Featured       bool
	Tags           string // comma list, see dishTags
	Popular        bool   // among the restaurant's best sellers (set by menu())
	Options        []Opt
	Groups         []OptGroup // Options arranged into their groups; free extras last
}

type Cat struct {
	ID             int64
	NameBn, NameEn string
	Items          []Item
}

type OItem struct {
	ID, MenuItemID                              int64
	NameBn, NameEn, Options, Note, Status, Last string
	Qty, ReqQty                                 int
	Unit, Line                                  int64
	CanChange, Direct                           bool
}

type OView struct {
	ID, SessionID                               int64
	N, Step, Minutes                            int
	Status, Reason, PlacedBy, ETA, Table, Group string
	Addon                                       bool
	PlacedAt, Est, ReadyAt, ServedAt            int64
	Items                                       []OItem
	Progress                                    int    // guest screen: 0 to 100 from placed to ready
	Age                                         string // kitchen screen: "ok", "warn" or "late" by waiting time
}

type BillLine struct {
	MenuItemID     int64
	NameBn, NameEn string
	Qty            int
	Amount         int64
}

type Bill struct {
	Lines                         []BillLine
	Subtotal, Service, VAT, Total int64
	Discount                      int64   // taken off the subtotal before service charge and VAT
	DiscountPercent               float64 // the discount as set: a percent, or 0 for a fixed amount
	DiscountReason, DiscountBy    string
	InvoiceNo                     int64 // given when paid; 0 before
}
