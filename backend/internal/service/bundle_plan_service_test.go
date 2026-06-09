//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ──────────────────────────────────────────────────────
// Noop base structs (panic on unexpected calls)
// ──────────────────────────────────────────────────────

type bundlePlanRepoNoop struct{}

func (bundlePlanRepoNoop) Create(context.Context, *BundlePlan) error {
	panic("unexpected Create call")
}
func (bundlePlanRepoNoop) Update(context.Context, *BundlePlan) error {
	panic("unexpected Update call")
}
func (bundlePlanRepoNoop) GetByID(context.Context, int64) (*BundlePlan, error) {
	panic("unexpected GetByID call")
}
func (bundlePlanRepoNoop) List(context.Context, pagination.PaginationParams, string, string) ([]BundlePlan, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (bundlePlanRepoNoop) ListForSale(context.Context) ([]BundlePlan, error) {
	panic("unexpected ListForSale call")
}
func (bundlePlanRepoNoop) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

// ──────────────────────────────────────────────────────
// Stubs for specific test scenarios
// ──────────────────────────────────────────────────────

// bundlePlanCreateStub supports Create + GetByID (used by CreatePlan flow).
type bundlePlanCreateStub struct {
	bundlePlanRepoNoop

	created *BundlePlan
	createErr error
}

func (s *bundlePlanCreateStub) Create(_ context.Context, plan *BundlePlan) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.created = plan
	plan.ID = 1
	return nil
}

// bundlePlanUpdateStub supports GetByID + Update.
type bundlePlanUpdateStub struct {
	bundlePlanRepoNoop

	existing  *BundlePlan
	updated   *BundlePlan
	updateErr error
}

func (s *bundlePlanUpdateStub) GetByID(_ context.Context, id int64) (*BundlePlan, error) {
	if s.existing == nil || s.existing.ID != id {
		return nil, ErrBundlePlanNotFound
	}
	cp := *s.existing
	return &cp, nil
}

func (s *bundlePlanUpdateStub) Update(_ context.Context, plan *BundlePlan) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updated = plan
	return nil
}

// bundlePlanListStub supports List + ListForSale.
type bundlePlanListStub struct {
	bundlePlanRepoNoop

	plans     []BundlePlan
	pagResult *pagination.PaginationResult
	listErr   error
}

func (s *bundlePlanListStub) List(_ context.Context, _ pagination.PaginationParams, _, _ string) ([]BundlePlan, *pagination.PaginationResult, error) {
	if s.listErr != nil {
		return nil, nil, s.listErr
	}
	return s.plans, s.pagResult, nil
}

func (s *bundlePlanListStub) ListForSale(_ context.Context) ([]BundlePlan, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.plans, nil
}

// bundlePlanGetStub supports GetByID only.
type bundlePlanGetStub struct {
	bundlePlanRepoNoop

	plan    *BundlePlan
	getErr  error
}

func (s *bundlePlanGetStub) GetByID(_ context.Context, id int64) (*BundlePlan, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.plan == nil || s.plan.ID != id {
		return nil, ErrBundlePlanNotFound
	}
	cp := *s.plan
	return &cp, nil
}

// ──────────────────────────────────────────────────────
// Helper constructors
// ──────────────────────────────────────────────────────

func newBundlePlanSvc(stub BundlePlanRepository) *BundlePlanService {
	return NewBundlePlanService(stub)
}

func samplePlan() *BundlePlan {
	return &BundlePlan{
		ID:               1,
		Name:             "Test Bundle",
		Tier:             BundleTierStarter,
		Price:            9.99,
		OriginalPrice:    19.99,
		Currency:         "USD",
		ValidityDays:     30,
		ConcurrencyLimit: 5,
		RPMLimit:         60,
		ForSale:          true,
		SortOrder:        0,
		Status:           domain.StatusActive,
		GroupQuotas: []BundlePlanGroupQuota{
			{GroupID: 10, QuotaScope: QuotaScopePlatform, DailyLimitUSD: 1.0, WeeklyLimitUSD: 5.0, MonthlyLimitUSD: 20.0},
		},
	}
}

// ──────────────────────────────────────────────────────
// Tests: CreatePlan
// ──────────────────────────────────────────────────────

func TestBundlePlanService_CreatePlan_Success(t *testing.T) {
	stub := &bundlePlanCreateStub{}
	svc := newBundlePlanSvc(stub)

	req := &CreateBundlePlanRequest{
		Name:         "Test Bundle",
		Tier:         BundleTierStarter,
		Price:        9.99,
		ValidityDays: 30,
		GroupQuotas: []CreateGroupQuotaRequest{
			{GroupID: 10, QuotaScope: QuotaScopePlatform, DailyLimitUSD: 1.0},
		},
	}

	plan, err := svc.CreatePlan(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Equal(t, int64(1), plan.ID)
	require.Equal(t, "Test Bundle", plan.Name)
	require.Equal(t, BundleTierStarter, plan.Tier)
	require.Len(t, plan.GroupQuotas, 1)
	require.Equal(t, int64(10), plan.GroupQuotas[0].GroupID)
	require.Equal(t, 1.0, plan.GroupQuotas[0].DailyLimitUSD)
}

func TestBundlePlanService_CreatePlan_NilRequest(t *testing.T) {
	svc := newBundlePlanSvc(&bundlePlanCreateStub{})

	plan, err := svc.CreatePlan(context.Background(), nil)

	require.Error(t, err)
	require.Nil(t, plan)
}

func TestBundlePlanService_CreatePlan_RepoError(t *testing.T) {
	stub := &bundlePlanCreateStub{createErr: errors.New("db error")}
	svc := newBundlePlanSvc(stub)

	req := &CreateBundlePlanRequest{
		Name:         "Test",
		Tier:         BundleTierStarter,
		ValidityDays: 30,
		GroupQuotas:  []CreateGroupQuotaRequest{{GroupID: 1}},
	}

	plan, err := svc.CreatePlan(context.Background(), req)

	require.Error(t, err)
	require.Nil(t, plan)
}

// ──────────────────────────────────────────────────────
// Tests: UpdatePlan
// ──────────────────────────────────────────────────────

func TestBundlePlanService_UpdatePlan_MergeScalarFields(t *testing.T) {
	existing := samplePlan()
	stub := &bundlePlanUpdateStub{existing: existing}
	svc := newBundlePlanSvc(stub)

	newName := "Updated Bundle"
	newPrice := 14.99
	req := &UpdateBundlePlanRequest{
		Name:  &newName,
		Price: &newPrice,
	}

	plan, err := svc.UpdatePlan(context.Background(), 1, req)

	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Equal(t, "Updated Bundle", stub.updated.Name)
	require.Equal(t, 14.99, stub.updated.Price)
	// Unchanged fields should remain.
	require.Equal(t, BundleTierStarter, stub.updated.Tier)
	require.Equal(t, 30, stub.updated.ValidityDays)
}

func TestBundlePlanService_UpdatePlan_ReplaceGroupQuotas(t *testing.T) {
	existing := samplePlan()
	stub := &bundlePlanUpdateStub{existing: existing}
	svc := newBundlePlanSvc(stub)

	newQuotas := []CreateGroupQuotaRequest{
		{GroupID: 20, QuotaScope: QuotaScopeModel, ModelPattern: "gpt-4*", DailyLimitUSD: 2.0},
		{GroupID: 30, QuotaScope: QuotaScopePlatform, MonthlyLimitUSD: 50.0},
	}
	req := &UpdateBundlePlanRequest{
		GroupQuotas: &newQuotas,
	}

	plan, err := svc.UpdatePlan(context.Background(), 1, req)

	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, stub.updated.GroupQuotas, 2)
	require.Equal(t, int64(20), stub.updated.GroupQuotas[0].GroupID)
	require.Equal(t, "gpt-4*", stub.updated.GroupQuotas[0].ModelPattern)
	require.Equal(t, int64(30), stub.updated.GroupQuotas[1].GroupID)
	require.Equal(t, 50.0, stub.updated.GroupQuotas[1].MonthlyLimitUSD)
}

func TestBundlePlanService_UpdatePlan_PlanNotFound(t *testing.T) {
	stub := &bundlePlanUpdateStub{existing: nil} // no plan
	svc := newBundlePlanSvc(stub)

	req := &UpdateBundlePlanRequest{Name: ptrStr("x")}

	plan, err := svc.UpdatePlan(context.Background(), 999, req)

	require.Error(t, err)
	require.Nil(t, plan)
}

func TestBundlePlanService_UpdatePlan_NilRequest(t *testing.T) {
	svc := newBundlePlanSvc(&bundlePlanUpdateStub{})

	plan, err := svc.UpdatePlan(context.Background(), 1, nil)

	require.Error(t, err)
	require.Nil(t, plan)
}

func TestBundlePlanService_UpdatePlan_RepoError(t *testing.T) {
	existing := samplePlan()
	stub := &bundlePlanUpdateStub{existing: existing, updateErr: errors.New("db error")}
	svc := newBundlePlanSvc(stub)

	req := &UpdateBundlePlanRequest{Name: ptrStr("x")}

	plan, err := svc.UpdatePlan(context.Background(), 1, req)

	require.Error(t, err)
	require.Nil(t, plan)
}

// ──────────────────────────────────────────────────────
// Tests: GetPlanDetail
// ──────────────────────────────────────────────────────

func TestBundlePlanService_GetPlanDetail_Success(t *testing.T) {
	expected := samplePlan()
	stub := &bundlePlanGetStub{plan: expected}
	svc := newBundlePlanSvc(stub)

	plan, err := svc.GetPlanDetail(context.Background(), 1)

	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Equal(t, int64(1), plan.ID)
	require.Equal(t, "Test Bundle", plan.Name)
}

func TestBundlePlanService_GetPlanDetail_NotFound(t *testing.T) {
	stub := &bundlePlanGetStub{plan: nil}
	svc := newBundlePlanSvc(stub)

	plan, err := svc.GetPlanDetail(context.Background(), 999)

	require.Error(t, err)
	require.Nil(t, plan)
}

// ──────────────────────────────────────────────────────
// Tests: ListPlans
// ──────────────────────────────────────────────────────

func TestBundlePlanService_ListPlans_Success(t *testing.T) {
	plans := []BundlePlan{{ID: 1, Name: "Basic"}, {ID: 2, Name: "Flagship"}}
	pagResult := &pagination.PaginationResult{Total: 2}
	stub := &bundlePlanListStub{plans: plans, pagResult: pagResult}
	svc := newBundlePlanSvc(stub)

	result, pag, err := svc.ListPlans(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 10}, "", "")

	require.NoError(t, err)
	require.Len(t, result, 2)
	require.NotNil(t, pag)
	require.Equal(t, int64(2), pag.Total)
}

func TestBundlePlanService_ListPlans_RepoError(t *testing.T) {
	stub := &bundlePlanListStub{listErr: errors.New("db error")}
	svc := newBundlePlanSvc(stub)

	result, pag, err := svc.ListPlans(context.Background(), pagination.DefaultPagination(), "", "")

	require.Error(t, err)
	require.Nil(t, result)
	require.Nil(t, pag)
}

// ──────────────────────────────────────────────────────
// Tests: ListForSale
// ──────────────────────────────────────────────────────

func TestBundlePlanService_ListForSale_Success(t *testing.T) {
	plans := []BundlePlan{{ID: 1, Name: "Basic", ForSale: true}}
	stub := &bundlePlanListStub{plans: plans}
	svc := newBundlePlanSvc(stub)

	result, err := svc.ListForSale(context.Background())

	require.NoError(t, err)
	require.Len(t, result, 1)
	require.True(t, result[0].ForSale)
}

func TestBundlePlanService_ListForSale_RepoError(t *testing.T) {
	stub := &bundlePlanListStub{listErr: errors.New("db error")}
	svc := newBundlePlanSvc(stub)

	result, err := svc.ListForSale(context.Background())

	require.Error(t, err)
	require.Nil(t, result)
}

// ──────────────────────────────────────────────────────
// Test helpers (ptrStr/ptrInt/ptrFloat defined in payment_config_plans_validation_test.go)
// ──────────────────────────────────────────────────────

func ptrBool(v bool) *bool { return &v }
