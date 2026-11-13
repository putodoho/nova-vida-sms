package billing

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

var cal2027 = Calendar{Year: 2027, BillingMonths: []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, DueDay: 5}

func d(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

// The client's statement: three monthly services from February, 5% flat VAT.
func TestGenerateSchoolStatement(t *testing.T) {
	fees := []Fee{
		{ID: uuid.New(), Name: "Mensalidade", Kind: "monthly", Amount: money.MT(2800), VATMode: money.ModeIncluded, VATRateBP: 500},
		{ID: uuid.New(), Name: "Extraclasse", Kind: "monthly", Amount: money.MT(1000), VATMode: money.ModeIncluded, VATRateBP: 500},
		{ID: uuid.New(), Name: "Almoço", Kind: "monthly", Amount: money.MT(1500), VATMode: money.ModeIncluded, VATRateBP: 500},
	}
	drafts, err := Generate(cal2027, 2, fees, money.VATIncludedFlat, d("2027-01-20"))
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 30 {
		t.Fatalf("got %d charges, want 30", len(drafts))
	}
	var gross, net, vat money.Amount
	for _, dr := range drafts {
		gross += dr.Lines.Gross
		net += dr.Lines.Net
		vat += dr.Lines.VAT
	}
	if gross != money.MT(53000) || vat != money.MT(2650) || net != money.MT(50350) {
		t.Fatalf("totals gross %s vat %s net %s", gross.Format(), vat.Format(), net.Format())
	}
	first := drafts[0]
	if first.DueDate != d("2027-02-05") || first.Description != "Mensalidade – Fevereiro 2027" || first.PeriodMonth != 2 {
		t.Fatalf("first charge: %+v", first)
	}
	if last := drafts[9]; last.DueDate != d("2027-11-05") {
		t.Fatalf("last month due: %v", last.DueDate)
	}
}

func TestGenerateMidYearAndOneOff(t *testing.T) {
	fees := []Fee{
		{ID: uuid.New(), Name: "Mensalidade Sala 2", Kind: "monthly", Amount: money.MT(3000), VATMode: money.ModeNone},
		{ID: uuid.New(), Name: "Livros", Kind: "one_off", Amount: money.MT(750), VATMode: money.ModeNone, Due: DueRule{Date: "2027-01-30"}},
		{ID: uuid.New(), Name: "Inscrição", Kind: "one_off", Amount: money.MT(3000), VATMode: money.ModeNone, Due: DueRule{OnEnrolment: true}},
	}
	// Enrolled on 10 May: monthly charges from June; books date already past → due on enrolment.
	drafts, err := Generate(cal2027, 6, fees, money.VATIncludedFlat, d("2027-05-10"))
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 6+1+1 {
		t.Fatalf("got %d charges", len(drafts))
	}
	if drafts[0].PeriodMonth != 6 {
		t.Fatalf("first month %d", drafts[0].PeriodMonth)
	}
	for _, dr := range drafts[6:] {
		if dr.DueDate != d("2027-05-10") || dr.PeriodMonth != 0 || dr.VATRateBP != 0 {
			t.Fatalf("one-off: %+v", dr)
		}
	}
	// Enrolled early: fixed date kept.
	drafts, _ = Generate(cal2027, 2, fees[1:2], money.VATIncludedFlat, d("2026-11-20"))
	if drafts[0].DueDate != d("2027-01-30") {
		t.Fatalf("books due %v", drafts[0].DueDate)
	}
}

func TestStatus(t *testing.T) {
	today := d("2027-03-10")
	cases := []struct {
		cancelled     bool
		payable, paid money.Amount
		due           string
		want          string
	}{
		{true, 100, 0, "2027-03-05", StatusCancelled},
		{false, 100, 100, "2027-03-05", StatusPaid},
		{false, 100, 0, "2027-03-05", StatusOverdue},
		{false, 100, 40, "2027-03-05", StatusOverdue},
		{false, 100, 40, "2027-04-05", StatusPartial},
		{false, 100, 0, "2027-03-10", StatusUpcoming},
		{false, 0, 0, "2027-03-05", StatusPaid}, // fully discounted
	}
	for _, c := range cases {
		if got := Status(c.cancelled, c.payable, c.paid, d(c.due), today); got != c.want {
			t.Errorf("%+v: got %s", c, got)
		}
	}
}
