// Response types of the API (mirrors the Go structs; see api/openapi.yaml).
// Money fields are integer centavos.

export type Permission =
  | 'pupils.read' | 'pupils.write' | 'health.read' | 'enrolments.write' | 'charges.read' | 'charges.cancel'
  | 'payments.create' | 'payments.cancel' | 'discounts.request' | 'discounts.approve' | 'penalties.waive'
  | 'documents.generate' | 'reports.read' | 'classes.read' | 'settings.manage' | 'users.manage' | 'audit.read'

export interface EntityRef { id: string; code: 'primary' | 'preschool'; short_name: string; legal_name: string }

export interface Me {
  user: { id: string; login: string; display_name: string; roles: string[]; permissions: Permission[]; entity_ids: string[] }
  entities: EntityRef[]
  today: string
}

export interface List<T> { items: T[]; total: number }

export interface SchoolYear { id: string; year: number; starts_on: string; ends_on: string; billing_months: number[]; due_day: number; status: 'planning' | 'open' | 'closed' }

export interface ClassItem { id: string; entity_id: string; name: string; grade: number | null; capacity: number; enrolled: number; teacher_name: string | null; teacher_staff_id: string | null; school_year_id: string; level_id: string; room: string }

export interface FeeItem { id: string; entity_id: string; school_year_id: string; code: string; name: string; kind: 'monthly' | 'one_off'; amount: number; vat_mode: 'none' | 'included' | 'added'; vat_rate_bp: number; mandatory: boolean; active: boolean; class_ids: string[]; due_rule: { on_enrolment?: boolean; date?: string } | null; lines: { net: number; vat: number; gross: number } }

export interface Account { id: string; entity_id: string; kind: 'cash' | 'bank' | 'mpesa' | 'emola'; label: string; holder: string; number: string; nib: string; active: boolean }

export interface EnrolmentListItem {
  enrolment_id: string; entity_id: string; pupil_id: string; number: string; full_name: string; birth_date: string
  class_id: string | null; class_name: string | null; status: 'draft' | 'confirmed' | 'withdrawn'; type: 'new' | 'returning'
  payer_name: string | null; payer_phone: string | null; health_alert: boolean; overdue?: number; outstanding?: number
}

export interface PupilSearchHit { id: string; number: string; full_name: string; birth_date: string; status: string; last_year: number | null; last_class: string | null; last_entity_id: string | null; payer_name: string | null; payer_phone: string | null }

export interface GuardianLink { guardian_id: string; full_name: string; phone1: string; phone2: string; email: string; neighbourhood: string; relationship: string; is_payer: boolean; receives_notices: boolean; may_pick_up: boolean; lives_with: boolean }

export interface Health { chronic_condition: string; allergies: string; diet_restrictions: string; medication: string; doctor_name: string; doctor_phone: string; notes: string; emergency_care_consent_at: string | null }

export interface PupilProfile {
  id: string; number: string; full_name: string; sex: 'F' | 'M'; birth_date: string; neighbourhood: string; religion: string
  orphan: 'none' | 'father' | 'mother' | 'both'; photo_path: string; home_discipline_notes: string; status: 'active' | 'left'
  health: Health | null; health_alert: boolean; guardians: GuardianLink[]
  pickup_persons: { full_name: string; phone: string; id_doc: string }[]
  emergency_contacts: { position: number; full_name: string; relationship: string; phone: string }[]
  enrolments: { id: string; entity_id: string; entity_name: string; year: number; class_name: string | null; type: string; status: string }[]
}

export interface AccountCharge {
  id: string; entity_id: string; kind: 'monthly' | 'one_off' | 'penalty'; period_month: number | null; description: string; due_date: string
  gross: number; vat: number; discount: number; payable: number; paid: number; remaining: number
  status: 'paid' | 'partial' | 'overdue' | 'upcoming' | 'cancelled'; parent_charge_id: string | null; cancel_reason?: string
}

export interface PupilAccount { pupil_id: string; year: number; summary: { overdue: number; outstanding: number; paid: number; credit: number; total: number }; charges: AccountCharge[] }

export interface Enrolment {
  id: string; entity_id: string; school_year_id: string; year: number; pupil_id: string; pupil_name: string; pupil_number: string
  class_id: string | null; class_name: string | null; type: 'new' | 'returning'; status: 'draft' | 'confirmed' | 'withdrawn'; start_month: number
  consents: { image_use: boolean; fees_agreement: boolean; uniform_books: boolean; emergency_care: boolean }
  services: { fee_item_id: string; code: string; name: string; amount: number; mandatory: boolean; from_month: number; to_month: number | null }[]
  one_off_fee_ids: string[]
  totals: { monthly_gross: number; monthly_net: number; monthly_vat: number; annual_gross: number; one_off_gross: number; months: number }
  preview: { description: string; kind: string; period_month: number; due_date: string; lines: { net: number; vat: number; gross: number } }[]
}

export interface AllocationLine { charge_id: string; description: string; due_date: string; kind: string; amount: number; active: boolean }

export interface Payment {
  id: string; entity_id: string; pupil_id: string; pupil_name: string; pupil_number: string; payment_group_id: string | null; receipt_no: string
  paid_on: string; amount: number; method: 'cash' | 'bank' | 'mpesa' | 'emola'; receiving_account_id: string | null; account_label: string | null
  reference: string; notes: string; received_by: string; received_by_name: string; created_at: string; cancelled_at: string | null; cancel_reason: string
  allocated: number; credit: number; allocations?: AllocationLine[]; pupil_overdue?: number; pupil_outstanding?: number
}

export interface PaymentPreview {
  open: { charge_id: string; description: string; due_date: string; kind: string; remaining: number; amount: number }[]
  allocations: { charge_id: string; description: string; due_date: string; kind: string; remaining: number; amount: number }[]
  credit: number; overdue: number; outstanding: number
}

export interface StatementRef { id: string; number: string; pupil_id: string; pupil_name: string; year: number; issued_at: string; issued_by: string }

export interface Dashboard {
  today: string; year: number; expected_month: number; received_month: number; collection_rate: number; overdue: number
  pupils_overdue: number; active_pupils: number
  monthly: { month: number; label: string; expected: number; received: number }[]
  by_method_month: Record<string, number>
  top_overdue: { pupil_id: string; name: string; class_name: string | null; overdue: number; days: number; phone: string | null }[]
  recent_payments: { id: string; receipt_no: string; pupil_name: string; amount: number; method: string; paid_on: string }[]
  pending_discounts: number
}

export interface Pupil { id: string; number: string; full_name: string; sex: 'F' | 'M'; birth_date: string; neighbourhood: string; religion: string; orphan: 'none' | 'father' | 'mother' | 'both'; photo_path: string; home_discipline_notes: string; status: 'active' | 'left' }

export interface Guardian { id: string; full_name: string; phone1: string; phone2: string; email: string; neighbourhood: string; id_doc: string }

export interface GuardianDetail extends Guardian { children: { pupil_id: string; number: string; full_name: string; relationship: string; is_payer: boolean }[] }

export interface RenewResult { pupil_id: string; enrolment_id: string | null; status: 'draft' | 'confirmed' | 'error'; error?: string }

export interface ChargeRow {
  id: string; entity_id: string; pupil_id: string; pupil_name: string; pupil_number: string; class_name: string | null; payer_phone: string | null
  kind: 'monthly' | 'one_off' | 'penalty'; period_month: number | null; description: string; due_date: string
  gross: number; discount: number; paid: number; remaining: number; days_overdue: number; status: 'paid' | 'partial' | 'overdue' | 'upcoming' | 'cancelled'
}
export interface Aging { d1_30: number; d31_60: number; d61_90: number; over_90: number; total: number; pupils: number }
export interface ChargeList { items: ChargeRow[]; total: number; aging: Aging }

export interface PupilArrears {
  pupil_id: string; entity_id: string; number: string; full_name: string; class_name: string | null
  payer_name: string | null; payer_phone: string | null; payer_phone2: string | null
  overdue: number; outstanding: number; charges: number; oldest_due: string; days_overdue: number
}

export interface DiscountType { id: string; entity_id: string; code: string; name: string; mode: 'percent' | 'fixed'; value: number; fee_item_codes: string[]; requires_approval: boolean; active: boolean }

export interface DiscountRequest {
  id: string; entity_id: string; enrolment_id: string; pupil_id: string; pupil_name: string; class_name: string | null
  discount_type_id: string; discount_type: string; fee_item_codes: string[]; mode: 'percent' | 'fixed'; value: number
  scope: { from_month: number }; reason: string; status: 'pending' | 'approved' | 'rejected'
  requested_by: string; decided_by: string | null; decided_at: string | null; decision_note: string; created_at: string
  /** pending: what approval would grant now; approved: what was granted */
  estimated_amount: number
}

export interface PaymentList { items: Payment[]; total: number; sum: number; by_method: Record<string, number> }

export interface ReportTable {
  title: string
  columns: { key: string; label: string; type: 'text' | 'int' | 'money' | 'date' | 'percent' }[]
  rows: (string | number | null)[][]
  totals?: (string | number | null)[]
}

export interface PenaltyRule { type: 'none' | 'percent' | 'fixed_tiers'; rate_bp?: number; grace_days: number; mode?: 'replace' | 'cumulative'; tiers?: { after_day: number; amount: number }[] }
export interface Level { id: string; entity_id: string; code: string; name: string; penalty_rule: PenaltyRule }

export interface Entity { id: string; code: string; legal_name: string; short_name: string; nuit: string; address: string; motto: string; logo_path: string; vat_method: string }

export interface Staff { id: string; full_name: string; function: string; phone: string; email: string; active: boolean }
export interface AppUser { id: string; staff_id: string | null; login: string; display_name: string; active: boolean; last_login_at: string | null; role_ids: string[]; role_codes: string[]; entity_ids: string[] }
export interface Role { id: string; code: string; name: string; description: string; system: boolean; permissions: string[]; users: number }
export interface PermissionInfo { code: string; module: string; label: string }
