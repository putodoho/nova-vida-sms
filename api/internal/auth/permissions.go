package auth

// Permission is a capability checked by the API. Permissions are fixed in
// code; roles (data) group them.
type Permission string

const (
	PupilsRead        Permission = "pupils.read"
	PupilsWrite       Permission = "pupils.write"
	HealthRead        Permission = "health.read"
	EnrolmentsWrite   Permission = "enrolments.write"
	ChargesRead       Permission = "charges.read"
	ChargesCancel     Permission = "charges.cancel"
	PaymentsCreate    Permission = "payments.create"
	PaymentsCancel    Permission = "payments.cancel"
	DiscountsRequest  Permission = "discounts.request"
	DiscountsApprove  Permission = "discounts.approve"
	PenaltiesWaive    Permission = "penalties.waive"
	DocumentsGenerate Permission = "documents.generate"
	ReportsRead       Permission = "reports.read"
	ClassesRead       Permission = "classes.read"
	SettingsManage    Permission = "settings.manage"
	UsersManage       Permission = "users.manage"
	AuditRead         Permission = "audit.read"
)

// PermissionInfo describes a permission for the roles editor.
type PermissionInfo struct {
	Code   Permission `json:"code"`
	Module string     `json:"module"`
	Label  string     `json:"label"` // Portuguese, shown in the UI
}

// All lists every permission in display order.
var All = []PermissionInfo{
	{PupilsRead, "Alunos", "Ver alunos e encarregados"},
	{PupilsWrite, "Alunos", "Criar e editar alunos e encarregados"},
	{HealthRead, "Alunos", "Ver dados de saúde"},
	{EnrolmentsWrite, "Matrículas", "Criar, confirmar e renovar matrículas"},
	{ChargesRead, "Financeiro", "Ver cobranças, saldos e dívidas"},
	{ChargesCancel, "Financeiro", "Anular cobranças"},
	{PaymentsCreate, "Financeiro", "Registar pagamentos"},
	{PaymentsCancel, "Financeiro", "Anular pagamentos"},
	{DiscountsRequest, "Financeiro", "Pedir descontos"},
	{DiscountsApprove, "Financeiro", "Aprovar descontos"},
	{PenaltiesWaive, "Financeiro", "Isentar multas"},
	{DocumentsGenerate, "Documentos", "Emitir recibos e extratos"},
	{ReportsRead, "Relatórios", "Ver relatórios e painel"},
	{ClassesRead, "Escola", "Ver turmas e listas de alunos"},
	{SettingsManage, "Administração", "Gerir configurações"},
	{UsersManage, "Administração", "Gerir utilizadores e funções"},
	{AuditRead, "Administração", "Ver auditoria"},
}

// IsValid reports whether p is a known permission.
func IsValid(p Permission) bool {
	for _, info := range All {
		if info.Code == p {
			return true
		}
	}
	return false
}

// SystemRole is a role created by the seed.
type SystemRole struct {
	Code        string
	Name        string
	Description string
	Permissions []Permission
}

// DefaultRoles are the four roles from the brief. Administrators get every
// permission.
func DefaultRoles() []SystemRole {
	all := make([]Permission, 0, len(All))
	for _, p := range All {
		all = append(all, p.Code)
	}
	return []SystemRole{
		{"admin", "Administrador", "Acesso total, incluindo configurações e utilizadores.", all},
		{"director", "Director", "Painel, relatórios, aprovações e consulta de registos.", []Permission{
			PupilsRead, HealthRead, ChargesRead, DiscountsApprove, PenaltiesWaive, ReportsRead, ClassesRead, AuditRead,
		}},
		{"reception", "Recepção / Secretaria", "Matrículas, registos de alunos, pagamentos e documentos.", []Permission{
			PupilsRead, PupilsWrite, HealthRead, EnrolmentsWrite, ChargesRead, PaymentsCreate,
			DiscountsRequest, DocumentsGenerate, ClassesRead,
		}},
		{"staff", "Funcionário", "Listas de turma, contactos e pessoas autorizadas a buscar.", []Permission{
			PupilsRead, HealthRead, ClassesRead,
		}},
	}
}
