package server

import (
	"log/slog"
	"net/http"
	"slices"
)

// Permission keys. Every staff route names the one it needs (see need), and the server refuses
// anything the person's role does not include, so a hidden button is never the only guard.
// docs/plans/owner-verification-admin-roles.md section 7 is the source of this table.
const (
	PermAccount     = "account.self"     // own logins, own language: everyone
	PermOwnPassword = "account.password" // change own password: owner only (staff passwords are set by the owner)
	PermOwnEmail    = "account.email"    // own email, for verification and "Forgot password"

	PermKitchenView   = "kitchen.view"
	PermOrderAccept   = "order.accept" // accept or reject a new order
	PermOrderReady    = "order.ready"
	PermOrderServe    = "order.serve"
	PermOrderUndo     = "order.undo" // step back any order; each role can still undo its own tap
	PermChangeRequest = "order.change_request"
	PermCallDone      = "call.done"
	PermStock         = "menu.sold_out"

	PermCounterView  = "bill.counter"
	PermBillView     = "bill.view"
	PermBillPaid     = "bill.mark_paid"
	PermBillVoid     = "bill.remove_item"
	PermBillClose    = "bill.close_unpaid"
	PermBillDiscount = "bill.discount" // a discount with a reason, recorded in the activity log
	PermTablesOrder  = "tables.order_for_guest"

	PermDashboard     = "dashboard.view"
	PermMenuEdit      = "menu.edit"
	PermTablesManage  = "tables.manage"
	PermStaffManage   = "staff.manage"
	PermActivity      = "activity.view"
	PermReports       = "reports.view"
	PermClosingReport = "reports.closing"
	PermSettings      = "settings.edit"
	PermNetworks      = "settings.network"
	PermProfile       = "branch.profile"
	PermBranches      = "branch.manage"         // Branches page, branch switcher, moving staff between branches: owner only
	PermCharges       = "settings.charges"      // VAT and service charge: owner only (split out of settings.edit)
	PermManagers      = "staff.manage_managers" // add, remove and set passwords of managers: owner only
	PermBusiness      = "business.profile"      // business name, logo, colour, tagline, and a branch's own: owner only
)

// allBranchPerms work in "All branches" mode: they are not about one branch. Every other route
// asks an owner in that mode to pick a branch first (see authFor).
var allBranchPerms = set(PermAccount, PermOwnPassword, PermOwnEmail, PermBranches, PermBusiness,
	PermDashboard, PermReports, PermClosingReport, PermActivity)

// rolePerms is exactly what each role may do. Anything not listed is refused.
var rolePerms = map[string]map[string]bool{
	"owner": set(PermAccount, PermOwnPassword, PermOwnEmail, PermKitchenView, PermOrderAccept, PermOrderReady, PermOrderServe, PermOrderUndo,
		PermChangeRequest, PermCallDone, PermStock, PermCounterView, PermBillView, PermBillPaid, PermBillVoid,
		PermBillClose, PermBillDiscount, PermTablesOrder, PermDashboard, PermMenuEdit, PermTablesManage, PermStaffManage,
		PermActivity, PermReports, PermClosingReport, PermSettings, PermNetworks, PermProfile, PermBranches, PermCharges, PermManagers, PermBusiness),
	// A manager runs one branch: everything the owner does there, except other branches, VAT and
	// service charge, managers and the owner, and their own email (docs/plans/multi-branch.md, 3).
	"manager": set(PermAccount, PermKitchenView, PermOrderAccept, PermOrderReady, PermOrderServe, PermOrderUndo,
		PermChangeRequest, PermCallDone, PermStock, PermCounterView, PermBillView, PermBillPaid, PermBillVoid,
		PermBillClose, PermBillDiscount, PermTablesOrder, PermDashboard, PermMenuEdit, PermTablesManage, PermStaffManage,
		PermActivity, PermReports, PermClosingReport, PermSettings, PermNetworks, PermProfile),
	"kitchen": set(PermAccount, PermKitchenView, PermOrderAccept, PermOrderReady, PermChangeRequest, PermStock),
	"waiter": set(PermAccount, PermKitchenView, PermOrderServe, PermChangeRequest, PermCallDone, PermStock,
		PermBillView, PermTablesOrder),
	"cashier": set(PermAccount, PermCounterView, PermBillView, PermBillPaid, PermClosingReport),
}

// staffRoles are the roles a person may give when adding someone. Owners are not on it: a
// business has one owner, and only the platform admin can change that.
var staffRoles = []string{"waiter", "kitchen", "cashier"}

// GivableRoles are the roles this person may give, and so also the people they may remove and
// set passwords for: the owner adds managers too, a manager only the service roles.
func (s *Staff) GivableRoles() []string {
	if s.Can(PermManagers) {
		return append([]string{"manager"}, staffRoles...)
	}
	return staffRoles
}

// CanManage reports whether this person may remove someone with role, or set their password.
func (s *Staff) CanManage(role string) bool { return slices.Contains(s.GivableRoles(), role) }

func set(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// Can reports whether this person's role allows the permission. Templates use it to show only
// the buttons that will work; the server checks again on every request.
func (s *Staff) Can(perm string) bool { return rolePerms[s.Role][perm] }

// need wraps a staff route: the person must be logged in and their role must include perm.
func (a *App) need(perm string, h func(http.ResponseWriter, *http.Request, *Staff)) http.HandlerFunc {
	if perm == "" {
		panic("need: every staff route must name a permission")
	}
	return a.authFor(perm, func(w http.ResponseWriter, r *http.Request, s *Staff) {
		if !s.Can(perm) {
			refuse(w, r, s, perm)
			return
		}
		h(w, r, s)
	})
}

// refuse answers a request the person's role does not allow, and logs it.
func refuse(w http.ResponseWriter, r *http.Request, s *Staff, perm string) {
	slog.Warn("refused", "staff", s.ID, "role", s.Role, "restaurant", s.R.ID, "perm", perm, "method", r.Method, "path", r.URL.Path)
	http.Error(w, "Your role can't do this. Ask the owner.", http.StatusForbidden)
}

// staffHome is the start page of a role: the first screen it can open.
func staffHome(role string) string {
	switch role {
	case "owner", "manager":
		return "/owner"
	case "cashier":
		return "/staff/counter"
	case "waiter":
		return "/staff/tables"
	}
	return "/staff/kitchen"
}

// Home is this person's start page, for links in templates.
func (s *Staff) Home() string { return staffHome(s.Role) }
