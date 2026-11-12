package server

type Restaurant struct {
	ID            int64
	Name          string
	VAT, Service  float64
	IsOpen        bool
	RequireAccept bool
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
	ID    int64
	Name  string
	Extra int64
}

type Item struct {
	ID, CatID      int64
	NameBn, NameEn string
	Price          int64
	Photo          string
	Prep           int
	Available      bool
	Options        []Opt
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
	PlacedAt, Est                               int64
	Items                                       []OItem
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
