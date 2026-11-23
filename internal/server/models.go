package server

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
	VerifyEmail                    string // global | required | off: must the owner confirm their email (set by the platform admin)
	OwnerVerified                  bool   // an owner of this restaurant has confirmed their email
	NotReady                       bool   // guests: closed because the owner hasn't confirmed their email yet
}

// restExtraCols are the restaurant columns read into the fields after Networks (see extraDest).
const restExtraCols = `r.open_time,r.close_time,r.last_order,r.warn_minutes,r.late_minutes,r.orders_in_hours,r.review_url,r.verify_email,
	EXISTS(SELECT 1 FROM staff_users o WHERE o.restaurant_id=r.id AND o.role='owner' AND o.email_verified_at>0)`

func (r *Restaurant) extraDest() []any {
	return []any{&r.OpenTime, &r.CloseTime, &r.LastOrder, &r.WarnMinutes, &r.LateMinutes, &r.OrdersInHours, &r.ReviewURL, &r.VerifyEmail, &r.OwnerVerified}
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
}
