"""Fill a running API with realistic demo data (dev only). Usage: python3 demo_data.py [http://localhost:8080/api/v1] — needs admin@novavida.co.mz / Secret123!."""
import json, random, sys, urllib.request, http.cookiejar
random.seed(7)
BASE = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/api/v1"
jar = http.cookiejar.CookieJar()
op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
ENT = {"h": None}

def call(method, path, body=None, ent=True):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if ent and ENT["h"]: req.add_header("X-Entity-ID", ENT["h"])
    try:
        with op.open(req) as r:
            t = r.read(); return json.loads(t) if t else None
    except urllib.error.HTTPError as e:
        raise SystemExit(f"{method} {path}: {e.code} {e.read().decode()}")

me = call("POST", "/auth/login", {"login": "admin@novavida.co.mz", "password": "Secret123!"}, ent=False)
ents = {e["code"]: e["id"] for e in me["entities"]}
years = call("GET", "/school-years", ent=False)["items"]
year = next(y for y in years if y["year"] == 2027)["id"]

first_f = ["Lúcia", "Ana", "Joana", "Celeste", "Graça", "Marta", "Isabel", "Rosa", "Esperança", "Fátima", "Dércia", "Nélia", "Shelsia", "Yolanda"]
first_m = ["Tiago", "Rui", "Carlos", "Edson", "Hélder", "Nelson", "Abel", "Faizal", "Délcio", "Osvaldo", "Kelvin", "Jaime", "Ivan", "Zacarias"]
last = ["Mabunda", "Cossa", "Machava", "Nhantumbo", "Sitoe", "Chissano", "Bila", "Mondlane", "Tembe", "Macuácua", "Langa", "Matsinhe", "Muianga", "Zandamela"]
mothers = ["Sónia", "Albertina", "Cristina", "Laura", "Telma", "Olga", "Benvinda", "Rita", "Amélia", "Ermelinda"]
hoods = ["Chinunguine", "Patrice Lumumba", "Inhamissa", "Marien Ngouabi", "Coca Missava", "Praia"]

def setup_accounts(code, holder):
    ENT["h"] = ents[code]
    call("POST", f"/entities/{ents[code]}/accounts", {"kind": "mpesa", "label": "M-Pesa", "holder": holder, "number": "84 589 2706"})
    call("POST", f"/entities/{ents[code]}/accounts", {"kind": "emola", "label": "e-Mola", "holder": holder, "number": "86 689 2706"})
    if code == "primary":
        call("POST", f"/entities/{ents[code]}/accounts", {"kind": "bank", "label": "Millennium BIM", "holder": "Escola Primária Nova Vida, SU, Lda.", "number": "1123380960", "nib": "000100000112338096057"})
    return {a["kind"]: a["id"] for a in call("GET", f"/entities/{ents[code]}/accounts")["items"]}

def make(code, n, months_paid_weights):
    accs = setup_accounts(code, "Escola Nova Vida")
    classes = call("GET", "/classes?year=2027")["items"]
    fees = call("GET", "/fee-items?year=2027")["items"]
    optional = [f for f in fees if f["kind"] == "monthly" and not f["mandatory"]]
    one_offs = [f for f in fees if f["kind"] == "one_off"]
    pupils = []
    for i in range(n):
        sex = random.choice("FM")
        name = f"{random.choice(first_f if sex == 'F' else first_m)} {random.choice(last)}"
        cls = classes[i % len(classes)]
        birth_year = (2021 if code == "preschool" else 2018) - (cls["grade"] or 0) + random.randint(0, 1)
        p = call("POST", "/pupils", {"full_name": name, "sex": sex, "birth_date": f"{birth_year}-{random.randint(1,12):02d}-{random.randint(1,28):02d}", "neighbourhood": random.choice(hoods), "religion": random.choice(["Cristã", "Católica", "Muçulmana", ""])})
        surname = name.split()[-1]
        call("PUT", f"/pupils/{p['id']}/guardians", [
            {"guardian": {"full_name": f"{random.choice(mothers)} {surname}", "phone1": f"84 {random.randint(100,999)} {random.randint(1000,9999)}", "neighbourhood": random.choice(hoods)}, "relationship": "mother", "is_payer": True, "receives_notices": True, "may_pick_up": True, "lives_with": True},
            {"guardian": {"full_name": f"{random.choice(['João','Pedro','Armando','Fernando','Alberto'])} {surname}", "phone1": f"82 {random.randint(100,999)} {random.randint(1000,9999)}"}, "relationship": "father", "is_payer": False, "receives_notices": False, "may_pick_up": True, "lives_with": random.random() > .2},
        ])
        if random.random() < .2:
            call("PUT", f"/pupils/{p['id']}/health", {"allergies": random.choice(["Amendoim", "Leite", "Penicilina", "Picada de abelha"]), "emergency_care_consent": True})
        call("PUT", f"/pupils/{p['id']}/emergency-contacts", [{"full_name": f"Avó {surname}", "relationship": "Avó", "phone": f"86 {random.randint(100,999)} {random.randint(1000,9999)}"}])
        opts = [f["id"] for f in optional if random.random() < .65 and (not f["class_ids"] or cls["id"] in f["class_ids"])]
        oo = [f["id"] for f in one_offs if (not f["class_ids"] or cls["id"] in f["class_ids"]) and f["code"] in ("enrolment", "uniform") + tuple(f"books_{x}" for x in ("sala1","sala2","sala3","pre"))]
        e = call("POST", "/enrolments", {"pupil_id": p["id"], "school_year_id": year, "class_id": cls["id"], "type": random.choice(["new", "returning"]), "monthly_fee_ids": opts, "one_off_fee_ids": oo, "consents": {"fees_agreement": True, "uniform_books": True, "image_use": random.random() > .3, "emergency_care": True}})
        e = call("POST", f"/enrolments/{e['id']}/confirm")
        pupils.append((p["id"], e["totals"]["monthly_gross"], e["totals"]["one_off_gross"]))
    # Payments Feb–May: each family pays a random number of months.
    days = ["2027-02-03", "2027-03-04", "2027-04-05", "2027-05-06"]
    for pid, monthly, one_off in pupils:
        months = random.choices([0, 1, 2, 3, 4], weights=months_paid_weights)[0]
        if one_off and random.random() < .8:
            call("POST", "/payments", {"pupil_id": pid, "amount": one_off, "paid_on": "2027-01-28", "method": "cash", "receiving_account_id": accs["cash"]})
        for m in range(months):
            method = random.choice(["cash", "cash", "mpesa", "mpesa", "emola", "bank"]) if "bank" in accs else random.choice(["cash", "mpesa", "emola"])
            amount = monthly if random.random() > .15 else monthly // 2
            ref = "" if method == "cash" else "".join(random.choices("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", k=9))
            call("POST", "/payments", {"pupil_id": pid, "amount": amount, "paid_on": days[m], "method": method, "receiving_account_id": accs[method], "reference": ref})
    call("POST", "/penalties/run")
    return pupils

prim = make("primary", 30, [1, 2, 4, 6, 8])
make("preschool", 14, [1, 2, 3, 5, 6])
# A pending discount request in the primary school.
ENT["h"] = ents["primary"]
sib = next(d for d in call("GET", "/discount-types")["items"] if d["code"] == "siblings")
call("PUT", f"/discount-types/{sib['id']}", {"name": "Irmãos", "mode": "percent", "value": 1000, "fee_item_codes": ["tuition_1_3", "tuition_4"], "requires_approval": True})
enr = call("GET", "/enrolments?page_size=1")["items"][0]
call("POST", "/discount-requests", {"enrolment_id": enr["enrolment_id"], "discount_type_id": sib["id"], "from_month": 6, "reason": "Dois irmãos na escola"})
print("demo data loaded")
