-- Initial schema for Nova Vida SMS.
-- Money columns are bigint centavos (1 MT = 100). See docs/technical-design.md.

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- Entities and settings
-- ---------------------------------------------------------------------------

CREATE TABLE entity (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code        text NOT NULL UNIQUE,              -- 'preschool', 'primary'
    legal_name  text NOT NULL,
    short_name  text NOT NULL,
    nuit        text NOT NULL DEFAULT '',
    address     text NOT NULL DEFAULT '',
    motto       text NOT NULL DEFAULT '',
    logo_path   text NOT NULL DEFAULT '',
    vat_method  text NOT NULL DEFAULT 'included_flat'
                CHECK (vat_method IN ('added', 'included_standard', 'included_flat')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE receiving_account (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id   uuid NOT NULL REFERENCES entity(id),
    kind        text NOT NULL CHECK (kind IN ('cash', 'bank', 'mpesa', 'emola')),
    label       text NOT NULL,
    holder      text NOT NULL DEFAULT '',
    number      text NOT NULL DEFAULT '',
    nib         text NOT NULL DEFAULT '',
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX receiving_account_entity_idx ON receiving_account(entity_id);

CREATE TABLE school_year (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    year            int NOT NULL UNIQUE,
    starts_on       date NOT NULL,
    ends_on         date NOT NULL,
    billing_months  int[] NOT NULL DEFAULT '{2,3,4,5,6,7,8,9,10,11}',
    due_day         int NOT NULL DEFAULT 5 CHECK (due_day BETWEEN 1 AND 28),
    status          text NOT NULL DEFAULT 'planning' CHECK (status IN ('planning', 'open', 'closed')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_on > starts_on)
);

CREATE TABLE level (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id     uuid NOT NULL REFERENCES entity(id),
    code          text NOT NULL CHECK (code IN ('preschool', 'primary', 'secondary')),
    name          text NOT NULL,
    penalty_rule  jsonb NOT NULL DEFAULT '{"type":"none"}',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entity_id, code)
);

-- ---------------------------------------------------------------------------
-- Staff, users and access control
-- ---------------------------------------------------------------------------

CREATE TABLE staff (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name   text NOT NULL,
    function    text NOT NULL DEFAULT '',
    phone       text NOT NULL DEFAULT '',
    email       text NOT NULL DEFAULT '',
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app_user (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    staff_id       uuid REFERENCES staff(id),
    login          citext NOT NULL UNIQUE,         -- email or phone
    display_name   text NOT NULL,
    password_hash  text NOT NULL,
    active         boolean NOT NULL DEFAULT true,
    last_login_at  timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE role (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code         text NOT NULL UNIQUE,
    name         text NOT NULL,
    description  text NOT NULL DEFAULT '',
    system       boolean NOT NULL DEFAULT false,   -- seeded roles cannot be deleted
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE role_permission (
    role_id     uuid NOT NULL REFERENCES role(id) ON DELETE CASCADE,
    permission  text NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE user_role (
    user_id  uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    role_id  uuid NOT NULL REFERENCES role(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE user_entity (
    user_id    uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    entity_id  uuid NOT NULL REFERENCES entity(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, entity_id)
);

CREATE TABLE session (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash  bytea NOT NULL UNIQUE,             -- sha256 of the cookie token
    user_id     uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    expires_at  timestamptz NOT NULL,
    ip          text NOT NULL DEFAULT '',
    user_agent  text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    last_seen   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX session_user_idx ON session(user_id);

-- ---------------------------------------------------------------------------
-- Classes and fee catalogue
-- ---------------------------------------------------------------------------

CREATE TABLE class (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id         uuid NOT NULL REFERENCES entity(id),
    level_id          uuid NOT NULL REFERENCES level(id),
    school_year_id    uuid NOT NULL REFERENCES school_year(id),
    name              text NOT NULL,              -- '1.ª A', 'Sala 1'
    grade             int CHECK (grade BETWEEN 1 AND 12),
    capacity          int NOT NULL DEFAULT 20 CHECK (capacity > 0),
    room              text NOT NULL DEFAULT '',
    teacher_staff_id  uuid REFERENCES staff(id),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entity_id, school_year_id, name)
);

CREATE TABLE fee_item (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id       uuid NOT NULL REFERENCES entity(id),
    school_year_id  uuid NOT NULL REFERENCES school_year(id),
    code            text NOT NULL,                -- 'tuition', 'after_school', 'lunch', 'enrolment', ...
    name            text NOT NULL,
    kind            text NOT NULL CHECK (kind IN ('monthly', 'one_off')),
    amount          bigint NOT NULL CHECK (amount >= 0),
    vat_mode        text NOT NULL DEFAULT 'none' CHECK (vat_mode IN ('none', 'included', 'added')),
    vat_rate_bp     int NOT NULL DEFAULT 0 CHECK (vat_rate_bp BETWEEN 0 AND 10000),
    due_rule        jsonb NOT NULL DEFAULT '{}',  -- one_off: {"date":"2027-01-30"} or {"on_enrolment":true}
    mandatory       boolean NOT NULL DEFAULT false,
    active          boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX fee_item_code_idx ON fee_item(entity_id, school_year_id, code);

-- A fee item applies to the listed classes; no rows = applies to every class of the entity/year.
CREATE TABLE fee_item_class (
    fee_item_id  uuid NOT NULL REFERENCES fee_item(id) ON DELETE CASCADE,
    class_id     uuid NOT NULL REFERENCES class(id) ON DELETE CASCADE,
    PRIMARY KEY (fee_item_id, class_id)
);

CREATE TABLE discount_type (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id          uuid NOT NULL REFERENCES entity(id),
    code               text NOT NULL,
    name               text NOT NULL,
    mode               text NOT NULL CHECK (mode IN ('percent', 'fixed')),
    value              bigint NOT NULL DEFAULT 0 CHECK (value >= 0),  -- basis points or centavos
    fee_item_codes     text[] NOT NULL DEFAULT '{tuition}',
    requires_approval  boolean NOT NULL DEFAULT true,
    active             boolean NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entity_id, code)
);

-- ---------------------------------------------------------------------------
-- Pupils and guardians (global, not entity-scoped)
-- ---------------------------------------------------------------------------

CREATE TABLE pupil (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number                 text NOT NULL UNIQUE,  -- NV-2027-0042
    full_name              text NOT NULL,
    sex                    text NOT NULL CHECK (sex IN ('F', 'M')),
    birth_date             date NOT NULL,
    neighbourhood          text NOT NULL DEFAULT '',
    religion               text NOT NULL DEFAULT '',
    orphan                 text NOT NULL DEFAULT 'none' CHECK (orphan IN ('none', 'father', 'mother', 'both')),
    photo_path             text NOT NULL DEFAULT '',
    home_discipline_notes  text NOT NULL DEFAULT '',
    status                 text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'left')),
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX pupil_name_idx ON pupil USING gin (to_tsvector('simple', full_name));

CREATE TABLE health_record (
    pupil_id                  uuid PRIMARY KEY REFERENCES pupil(id) ON DELETE CASCADE,
    chronic_condition         text NOT NULL DEFAULT '',
    allergies                 text NOT NULL DEFAULT '',
    diet_restrictions         text NOT NULL DEFAULT '',
    medication                text NOT NULL DEFAULT '',
    doctor_name               text NOT NULL DEFAULT '',
    doctor_phone              text NOT NULL DEFAULT '',
    notes                     text NOT NULL DEFAULT '',
    emergency_care_consent_at timestamptz,
    updated_at                timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE guardian (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name      text NOT NULL,
    phone1         text NOT NULL DEFAULT '',
    phone2         text NOT NULL DEFAULT '',
    email          text NOT NULL DEFAULT '',
    neighbourhood  text NOT NULL DEFAULT '',
    id_doc         text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX guardian_phone_idx ON guardian(phone1);

CREATE TABLE pupil_guardian (
    pupil_id          uuid NOT NULL REFERENCES pupil(id) ON DELETE CASCADE,
    guardian_id       uuid NOT NULL REFERENCES guardian(id),
    relationship      text NOT NULL CHECK (relationship IN ('mother', 'father', 'guardian', 'other')),
    is_payer          boolean NOT NULL DEFAULT false,
    receives_notices  boolean NOT NULL DEFAULT false,
    may_pick_up       boolean NOT NULL DEFAULT true,
    lives_with        boolean NOT NULL DEFAULT true,
    PRIMARY KEY (pupil_id, guardian_id)
);
CREATE UNIQUE INDEX pupil_one_payer_idx ON pupil_guardian(pupil_id) WHERE is_payer;

CREATE TABLE pickup_person (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pupil_id   uuid NOT NULL REFERENCES pupil(id) ON DELETE CASCADE,
    full_name  text NOT NULL,
    phone      text NOT NULL DEFAULT '',
    id_doc     text NOT NULL DEFAULT ''
);

CREATE TABLE emergency_contact (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pupil_id      uuid NOT NULL REFERENCES pupil(id) ON DELETE CASCADE,
    position      int NOT NULL CHECK (position BETWEEN 1 AND 5),
    full_name     text NOT NULL,
    relationship  text NOT NULL DEFAULT '',
    phone         text NOT NULL,
    UNIQUE (pupil_id, position)
);

-- ---------------------------------------------------------------------------
-- Enrolment and ledger (entity-scoped)
-- ---------------------------------------------------------------------------

CREATE TABLE enrolment (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id       uuid NOT NULL REFERENCES entity(id),
    school_year_id  uuid NOT NULL REFERENCES school_year(id),
    pupil_id        uuid NOT NULL REFERENCES pupil(id),
    class_id        uuid REFERENCES class(id),
    type            text NOT NULL CHECK (type IN ('new', 'returning')),
    status          text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'confirmed', 'withdrawn')),
    start_month     int NOT NULL DEFAULT 2 CHECK (start_month BETWEEN 1 AND 12),
    consents        jsonb NOT NULL DEFAULT '{}',
    draft_data      jsonb NOT NULL DEFAULT '{}',  -- wizard state while in draft
    confirmed_at    timestamptz,
    withdrawn_at    timestamptz,
    created_by      uuid REFERENCES app_user(id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX enrolment_one_per_year_idx ON enrolment(pupil_id, school_year_id) WHERE status <> 'draft';
CREATE INDEX enrolment_scope_idx ON enrolment(entity_id, school_year_id);

CREATE TABLE contracted_service (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    enrolment_id  uuid NOT NULL REFERENCES enrolment(id) ON DELETE CASCADE,
    fee_item_id   uuid NOT NULL REFERENCES fee_item(id),
    from_month    int NOT NULL CHECK (from_month BETWEEN 1 AND 12),
    to_month      int CHECK (to_month BETWEEN 1 AND 12),   -- null = until year end
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE charge (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id         uuid NOT NULL REFERENCES entity(id),
    enrolment_id      uuid NOT NULL REFERENCES enrolment(id),
    pupil_id          uuid NOT NULL REFERENCES pupil(id),
    fee_item_id       uuid REFERENCES fee_item(id),
    kind              text NOT NULL CHECK (kind IN ('monthly', 'one_off', 'penalty')),
    parent_charge_id  uuid REFERENCES charge(id),
    rule_tier         int,                         -- penalty tier, for idempotency
    period_month      int CHECK (period_month BETWEEN 1 AND 12),
    description       text NOT NULL,
    due_date          date NOT NULL,
    amount_net        bigint NOT NULL CHECK (amount_net >= 0),
    amount_vat        bigint NOT NULL DEFAULT 0 CHECK (amount_vat >= 0),
    amount_gross      bigint NOT NULL CHECK (amount_gross >= 0),
    vat_rate_bp       int NOT NULL DEFAULT 0,
    cancelled_at      timestamptz,
    cancelled_by      uuid REFERENCES app_user(id),
    cancel_reason     text NOT NULL DEFAULT '',
    created_by        uuid REFERENCES app_user(id),
    created_at        timestamptz NOT NULL DEFAULT now(),
    CHECK (amount_gross = amount_net + amount_vat),
    CHECK ((kind = 'penalty') = (parent_charge_id IS NOT NULL))
);
CREATE INDEX charge_pupil_idx ON charge(entity_id, pupil_id, due_date);
CREATE INDEX charge_due_idx ON charge(entity_id, due_date) WHERE cancelled_at IS NULL;
CREATE UNIQUE INDEX charge_penalty_tier_idx ON charge(parent_charge_id, rule_tier) WHERE kind = 'penalty';

CREATE TABLE discount_request (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id         uuid NOT NULL REFERENCES entity(id),
    enrolment_id      uuid NOT NULL REFERENCES enrolment(id),
    discount_type_id  uuid NOT NULL REFERENCES discount_type(id),
    mode              text NOT NULL CHECK (mode IN ('percent', 'fixed')),
    value             bigint NOT NULL CHECK (value > 0),
    scope             jsonb NOT NULL DEFAULT '{}',
    reason            text NOT NULL,
    status            text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    requested_by      uuid NOT NULL REFERENCES app_user(id),
    decided_by        uuid REFERENCES app_user(id),
    decided_at        timestamptz,
    decision_note     text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE discount (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    charge_id            uuid NOT NULL REFERENCES charge(id),
    discount_request_id  uuid NOT NULL REFERENCES discount_request(id),
    amount               bigint NOT NULL CHECK (amount > 0),
    created_at           timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE payment (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id             uuid NOT NULL REFERENCES entity(id),
    pupil_id              uuid NOT NULL REFERENCES pupil(id),
    payment_group_id      uuid,
    receipt_no            text NOT NULL,
    paid_on               date NOT NULL,
    amount                bigint NOT NULL CHECK (amount > 0),
    method                text NOT NULL CHECK (method IN ('cash', 'bank', 'mpesa', 'emola')),
    receiving_account_id  uuid REFERENCES receiving_account(id),
    reference             text NOT NULL DEFAULT '',
    notes                 text NOT NULL DEFAULT '',
    received_by           uuid NOT NULL REFERENCES app_user(id),
    cancelled_at          timestamptz,
    cancelled_by          uuid REFERENCES app_user(id),
    cancel_reason         text NOT NULL DEFAULT '',
    created_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entity_id, receipt_no),
    CHECK (method = 'cash' OR reference <> '')
);
CREATE INDEX payment_pupil_idx ON payment(entity_id, pupil_id);
CREATE INDEX payment_date_idx ON payment(entity_id, paid_on);

CREATE TABLE allocation (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id  uuid NOT NULL REFERENCES payment(id),
    charge_id   uuid NOT NULL REFERENCES charge(id),
    amount      bigint NOT NULL CHECK (amount > 0),
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX allocation_charge_idx ON allocation(charge_id) WHERE active;
CREATE INDEX allocation_payment_idx ON allocation(payment_id);

CREATE TABLE statement (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id       uuid NOT NULL REFERENCES entity(id),
    pupil_id        uuid NOT NULL REFERENCES pupil(id),
    school_year_id  uuid NOT NULL REFERENCES school_year(id),
    number          text NOT NULL,
    issued_at       timestamptz NOT NULL DEFAULT now(),
    issued_by       uuid NOT NULL REFERENCES app_user(id),
    pdf_path        text NOT NULL DEFAULT '',
    snapshot        jsonb NOT NULL,
    UNIQUE (entity_id, number)
);

CREATE TABLE number_series (
    entity_id   uuid NOT NULL REFERENCES entity(id),
    series      text NOT NULL,                     -- 'receipt', 'statement'
    year        int NOT NULL,
    prefix      text NOT NULL,                     -- 'REC', 'EXT'
    next_value  int NOT NULL DEFAULT 1,
    PRIMARY KEY (entity_id, series, year)
);

-- Global pupil numbers: NV-<year>-<seq>
CREATE TABLE pupil_number_series (
    year        int PRIMARY KEY,
    next_value  int NOT NULL DEFAULT 1
);

-- ---------------------------------------------------------------------------
-- Audit and imports
-- ---------------------------------------------------------------------------

CREATE TABLE audit_log (
    id          bigserial PRIMARY KEY,
    at          timestamptz NOT NULL DEFAULT now(),
    user_id     uuid REFERENCES app_user(id),
    entity_id   uuid REFERENCES entity(id),
    action      text NOT NULL,                     -- 'create', 'update', 'cancel', 'login', ...
    table_name  text NOT NULL,
    record_id   text NOT NULL,
    before      jsonb,
    after       jsonb,
    ip          text NOT NULL DEFAULT ''
);
CREATE INDEX audit_log_record_idx ON audit_log(table_name, record_id);
CREATE INDEX audit_log_at_idx ON audit_log(at DESC);

CREATE TABLE import_job (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind        text NOT NULL CHECK (kind IN ('pupils', 'payments')),
    file_path   text NOT NULL,
    status      text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'validated', 'applied', 'failed')),
    report      jsonb NOT NULL DEFAULT '{}',
    created_by  uuid NOT NULL REFERENCES app_user(id),
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- updated_at triggers
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['entity','receiving_account','school_year','level','staff','app_user','role',
                             'class','fee_item','discount_type','pupil','guardian','enrolment']
    LOOP
        EXECUTE format('CREATE TRIGGER %I_updated_at BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION set_updated_at()', t, t);
    END LOOP;
END $$;
