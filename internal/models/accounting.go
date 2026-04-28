package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Income 收入記錄
type Income struct {
	ID             uuid.UUID     `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID     `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant         Tenant        `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	RelatedUserID  *uuid.UUID    `gorm:"type:uuid" json:"related_user_id,omitempty"`
	IncomeType     string        `gorm:"type:varchar(50);not null" json:"income_type"` // order, invoice, service, other
	ReferenceID    *uuid.UUID    `gorm:"type:uuid" json:"reference_id,omitempty"`      // 關聯的訂單ID、發票ID等
	ReferenceType  string        `gorm:"type:varchar(50)" json:"reference_type"`       // order, invoice, service_order
	Category       string        `gorm:"type:varchar(100)" json:"category"`            // 收入類別
	Description    string        `gorm:"type:text" json:"description"`
	Amount         float64       `gorm:"type:decimal(15,2);not null" json:"amount"`
	IncomeDate     time.Time     `gorm:"type:date;not null" json:"income_date"`
	PaymentMethod  string        `gorm:"type:varchar(50)" json:"payment_method"`     // cash, bank_transfer, credit_card, etc.
	BankAccountID  *uuid.UUID    `gorm:"type:uuid" json:"bank_account_id,omitempty"` // 收款賬戶(可選輸入)
	BankAccount    *BankAccount  `gorm:"foreignKey:BankAccountID" json:"bank_account,omitempty"`
	Status         string        `gorm:"type:varchar(50);default:'confirmed'" json:"status"` // confirmed, pending, cancelled
	JournalEntryID *uuid.UUID    `gorm:"type:uuid" json:"journal_entry_id,omitempty"`
	JournalEntry   *JournalEntry `gorm:"foreignKey:JournalEntryID" json:"journal_entry,omitempty"`
	Notes          string        `gorm:"type:text" json:"notes"`
	CreatedBy      *uuid.UUID    `gorm:"type:uuid" json:"created_by"`
	UpdatedBy      *uuid.UUID    `gorm:"type:uuid" json:"updated_by"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	TrashedAt      *time.Time    `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
	ExtraFields    JSONB         `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`

	// 關聯對象（通過 Preload 加載，不映射到數據庫）
	Order        *Order        `gorm:"-" json:"order,omitempty"`
	ServiceOrder *ServiceOrder `gorm:"-" json:"service_order,omitempty"`
}

func (Income) TableName() string {
	return "incomes"
}

// BeforeCreate 創建前設置 UUID
func (i *Income) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	return nil
}

// Expense 支出記錄
type Expense struct {
	ID             uuid.UUID     `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID     `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant         Tenant        `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	ProjectID      *uuid.UUID    `gorm:"type:uuid" json:"project_id,omitempty"`
	RelatedUserID  *uuid.UUID    `gorm:"type:uuid" json:"related_user_id,omitempty"`
	ExpenseType    string        `gorm:"type:varchar(50);not null" json:"expense_type"` // purchase, salary, rent, utility, other
	ReferenceID    *uuid.UUID    `gorm:"type:uuid" json:"reference_id,omitempty"`       // 關聯的採購單ID等
	ReferenceType  string        `gorm:"type:varchar(50)" json:"reference_type"`        // purchase_order, etc.
	Category       string        `gorm:"type:varchar(100);not null" json:"category"`    // 支出類別
	Description    string        `gorm:"type:text" json:"description"`
	Amount         float64       `gorm:"type:decimal(15,2);not null" json:"amount"`
	ExpenseDate    time.Time     `gorm:"type:date;not null" json:"expense_date"`
	PaymentMethod  string        `gorm:"type:varchar(50)" json:"payment_method"`     // cash, bank_transfer, credit_card, etc.
	BankAccountID  *uuid.UUID    `gorm:"type:uuid" json:"bank_account_id,omitempty"` // 付款賬戶(可選輸入)
	BankAccount    *BankAccount  `gorm:"foreignKey:BankAccountID" json:"bank_account,omitempty"`
	Vendor         string        `gorm:"type:varchar(255)" json:"vendor"`                    // 供應商/收款方
	Status         string        `gorm:"type:varchar(50);default:'confirmed'" json:"status"` // confirmed, pending, cancelled
	JournalEntryID *uuid.UUID    `gorm:"type:uuid" json:"journal_entry_id,omitempty"`
	JournalEntry   *JournalEntry `gorm:"foreignKey:JournalEntryID" json:"journal_entry,omitempty"`
	Notes          string        `gorm:"type:text" json:"notes"`
	CreatedBy      *uuid.UUID    `gorm:"type:uuid" json:"created_by"`
	UpdatedBy      *uuid.UUID    `gorm:"type:uuid" json:"updated_by"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	TrashedAt      *time.Time    `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
	ExtraFields    JSONB         `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`

	// 關聯對象（通過 Preload 加載，不映射到數據庫）
	Order         *Order         `gorm:"-" json:"order,omitempty"`
	ServiceOrder  *ServiceOrder  `gorm:"-" json:"service_order,omitempty"`
	PurchaseOrder *PurchaseOrder `gorm:"-" json:"purchase_order,omitempty"`
}

func (Expense) TableName() string {
	return "expenses"
}

// BeforeCreate 創建前設置 UUID
func (e *Expense) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// ExpenseRequest 支出申請
type ExpenseRequest struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Title       string     `gorm:"type:varchar(255);not null" json:"title"`
	Amount      float64    `gorm:"type:decimal(15,2);not null;default:0" json:"amount"`
	Description string     `gorm:"type:text" json:"description"`
	RequestDate time.Time  `gorm:"type:date;not null" json:"request_date"`
	Status      string     `gorm:"type:varchar(20);default:'pending'" json:"status"` // pending, approved, rejected
	ExpenseID   *uuid.UUID `gorm:"type:uuid" json:"expense_id"`
	ApprovedBy  *uuid.UUID `gorm:"type:uuid" json:"approved_by"`
	ApprovedAt  *time.Time `json:"approved_at"`
	CreatedBy   *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	Expense  *Expense `gorm:"foreignKey:ExpenseID" json:"expense,omitempty"`
	Approver *User    `gorm:"foreignKey:ApprovedBy" json:"approver,omitempty"`
}

func (ExpenseRequest) TableName() string {
	return "expense_requests"
}

// ============================================
// 會計科目表 (Chart of Accounts)
// ============================================

// AccountType 會計科目類型
// asset=資產, liability=負債, equity=權益, revenue=收入, expense=費用
type Account struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	ParentID    *uuid.UUID `gorm:"type:uuid" json:"parent_id,omitempty"`          // 上級科目（樹狀結構）
	Code        string     `gorm:"type:varchar(20);not null" json:"code"`         // 科目代碼 e.g. 1100, 2100, 4100
	Name        string     `gorm:"type:varchar(255);not null" json:"name"`        // 科目名稱
	AccountType string     `gorm:"type:varchar(20);not null" json:"account_type"` // asset, liability, equity, revenue, expense
	SubType     string     `gorm:"type:varchar(50)" json:"sub_type"`              // current_asset, fixed_asset, current_liability, long_term_liability, operating_revenue, cogs, operating_expense, other_income, other_expense, tax_expense
	Description string     `gorm:"type:text" json:"description"`
	Currency    string     `gorm:"type:varchar(10);default:''" json:"currency"` // 幣別（空=跟隨租戶設定）
	IsSystem    bool       `gorm:"default:false" json:"is_system"`              // 系統預設科目（不可刪除）
	IsActive    bool       `gorm:"default:true" json:"is_active"`               // 是否啟用
	TaxRate     float64    `gorm:"type:decimal(8,4);default:0" json:"tax_rate"` // 預設稅率（百分比）
	SortOrder   int        `gorm:"default:0" json:"sort_order"`                 // 排序
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	// 關聯
	Parent   *Account  `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Children []Account `gorm:"foreignKey:ParentID" json:"children,omitempty"`
}

func (Account) TableName() string { return "accounts" }

func (a *Account) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

// ============================================
// 日記帳 / 總帳 (Journal Entries)
// ============================================

// JournalEntry 日記帳分錄（一筆交易的表頭）
type JournalEntry struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	EntryNumber   string     `gorm:"type:varchar(50);not null" json:"entry_number"` // 分錄編號 JE-20260315-001
	EntryDate     time.Time  `gorm:"type:date;not null" json:"entry_date"`
	Description   string     `gorm:"type:text" json:"description"`
	ReferenceType string     `gorm:"type:varchar(50)" json:"reference_type"`          // income, expense, invoice, purchase, manual
	ReferenceID   *uuid.UUID `gorm:"type:uuid" json:"reference_id,omitempty"`         // 關聯的來源記錄
	Status        string     `gorm:"type:varchar(20);default:'posted'" json:"status"` // draft, posted, void
	TotalDebit    float64    `gorm:"type:decimal(15,2);default:0" json:"total_debit"`
	TotalCredit   float64    `gorm:"type:decimal(15,2);default:0" json:"total_credit"`
	CreatedBy     *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy     *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// 關聯
	Lines []JournalEntryLine `gorm:"foreignKey:JournalEntryID" json:"lines,omitempty"`
}

func (JournalEntry) TableName() string { return "journal_entries" }

func (j *JournalEntry) BeforeCreate(tx *gorm.DB) error {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	return nil
}

// JournalEntryLine 日記帳明細（借方/貸方行）
type JournalEntryLine struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	JournalEntryID uuid.UUID `gorm:"type:uuid;not null;index" json:"journal_entry_id"`
	AccountID      uuid.UUID `gorm:"type:uuid;not null" json:"account_id"` // 會計科目
	Description    string    `gorm:"type:text" json:"description"`
	DebitAmount    float64   `gorm:"type:decimal(15,2);default:0" json:"debit_amount"`
	CreditAmount   float64   `gorm:"type:decimal(15,2);default:0" json:"credit_amount"`
	CreatedAt      time.Time `json:"created_at"`

	// 關聯
	Account      *Account      `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	JournalEntry *JournalEntry `gorm:"foreignKey:JournalEntryID" json:"journal_entry,omitempty"`
}

func (JournalEntryLine) TableName() string { return "journal_entry_lines" }

func (l *JournalEntryLine) BeforeCreate(tx *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	return nil
}

// ============================================
// 稅務設定 (Tax Configuration)
// ============================================

// TaxConfig 稅務配置（支援多地區稅制）
type TaxConfig struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name        string     `gorm:"type:varchar(100);not null" json:"name"`    // 例：營業稅、利得稅、GST、VAT
	Code        string     `gorm:"type:varchar(20)" json:"code"`              // 例：VAT, GST, ST
	Region      string     `gorm:"type:varchar(50)" json:"region"`            // 地區：TW, HK, SG, US, etc.
	TaxType     string     `gorm:"type:varchar(30);not null" json:"tax_type"` // sales_tax, purchase_tax, income_tax, vat
	Rate        float64    `gorm:"type:decimal(8,4);not null" json:"rate"`    // 稅率百分比
	IsDefault   bool       `gorm:"default:false" json:"is_default"`           // 是否為該類型預設
	IsActive    bool       `gorm:"default:true" json:"is_active"`
	AccountID   *uuid.UUID `gorm:"type:uuid" json:"account_id,omitempty"` // 對應的稅務科目
	Description string     `gorm:"type:text" json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	Account *Account `gorm:"foreignKey:AccountID" json:"account,omitempty"`
}

func (TaxConfig) TableName() string { return "tax_configs" }

func (t *TaxConfig) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

// PostingRule 自動過帳規則（收入/支出 -> 借貸科目）
type PostingRule struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	SourceType      string    `gorm:"type:varchar(20);not null" json:"source_type"` // income, expense
	Category        string    `gorm:"type:varchar(100);not null" json:"category"`   // order, rent, other... / *
	DebitAccountID  uuid.UUID `gorm:"type:uuid;not null" json:"debit_account_id"`
	CreditAccountID uuid.UUID `gorm:"type:uuid;not null" json:"credit_account_id"`
	Description     string    `gorm:"type:text" json:"description"`
	IsSystem        bool      `gorm:"default:false" json:"is_system"`
	IsActive        bool      `gorm:"default:true" json:"is_active"`
	SortOrder       int       `gorm:"default:0" json:"sort_order"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	DebitAccount  *Account `gorm:"foreignKey:DebitAccountID" json:"debit_account,omitempty"`
	CreditAccount *Account `gorm:"foreignKey:CreditAccountID" json:"credit_account,omitempty"`
}

func (PostingRule) TableName() string { return "posting_rules" }

func (p *PostingRule) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

// ============================================
// 預設會計科目表定義
// ============================================

// DefaultAccount 用於初始化預設科目
type DefaultAccount struct {
	Code        string
	Name        string
	NameEN      string
	AccountType string
	SubType     string
	SortOrder   int
	TaxRate     float64
	Children    []DefaultAccount
}

// GetDefaultChartOfAccounts 返回預設會計科目表（通用版本，適用多地區）
func GetDefaultChartOfAccounts() []DefaultAccount {
	return []DefaultAccount{
		// ===== 資產 (Assets) 1xxx =====
		{Code: "1000", Name: "資產", NameEN: "Assets", AccountType: "asset", SubType: "", SortOrder: 100, Children: []DefaultAccount{
			{Code: "1100", Name: "流動資產", NameEN: "Current Assets", AccountType: "asset", SubType: "current_asset", SortOrder: 110, Children: []DefaultAccount{
				{Code: "1110", Name: "現金及銀行存款", NameEN: "Cash and Bank", AccountType: "asset", SubType: "current_asset", SortOrder: 111},
				{Code: "1120", Name: "應收帳款", NameEN: "Accounts Receivable", AccountType: "asset", SubType: "current_asset", SortOrder: 112},
				{Code: "1130", Name: "存貨", NameEN: "Inventory", AccountType: "asset", SubType: "current_asset", SortOrder: 113},
				{Code: "1140", Name: "預付費用", NameEN: "Prepaid Expenses", AccountType: "asset", SubType: "current_asset", SortOrder: 114},
				{Code: "1150", Name: "進項稅額", NameEN: "Input Tax Credit", AccountType: "asset", SubType: "current_asset", SortOrder: 115},
				{Code: "1190", Name: "其他流動資產", NameEN: "Other Current Assets", AccountType: "asset", SubType: "current_asset", SortOrder: 119},
			}},
			{Code: "1200", Name: "非流動資產", NameEN: "Non-Current Assets", AccountType: "asset", SubType: "fixed_asset", SortOrder: 120, Children: []DefaultAccount{
				{Code: "1210", Name: "固定資產", NameEN: "Property, Plant & Equipment", AccountType: "asset", SubType: "fixed_asset", SortOrder: 121},
				{Code: "1220", Name: "累計折舊", NameEN: "Accumulated Depreciation", AccountType: "asset", SubType: "fixed_asset", SortOrder: 122},
				{Code: "1230", Name: "無形資產", NameEN: "Intangible Assets", AccountType: "asset", SubType: "fixed_asset", SortOrder: 123},
				{Code: "1290", Name: "其他非流動資產", NameEN: "Other Non-Current Assets", AccountType: "asset", SubType: "fixed_asset", SortOrder: 129},
			}},
		}},
		// ===== 負債 (Liabilities) 2xxx =====
		{Code: "2000", Name: "負債", NameEN: "Liabilities", AccountType: "liability", SubType: "", SortOrder: 200, Children: []DefaultAccount{
			{Code: "2100", Name: "流動負債", NameEN: "Current Liabilities", AccountType: "liability", SubType: "current_liability", SortOrder: 210, Children: []DefaultAccount{
				{Code: "2110", Name: "應付帳款", NameEN: "Accounts Payable", AccountType: "liability", SubType: "current_liability", SortOrder: 211},
				{Code: "2120", Name: "應付薪資", NameEN: "Accrued Payroll", AccountType: "liability", SubType: "current_liability", SortOrder: 212},
				{Code: "2130", Name: "銷項稅額", NameEN: "Output Tax Payable", AccountType: "liability", SubType: "current_liability", SortOrder: 213},
				{Code: "2140", Name: "應付所得稅", NameEN: "Income Tax Payable", AccountType: "liability", SubType: "current_liability", SortOrder: 214},
				{Code: "2150", Name: "預收款項", NameEN: "Unearned Revenue", AccountType: "liability", SubType: "current_liability", SortOrder: 215},
				{Code: "2190", Name: "其他流動負債", NameEN: "Other Current Liabilities", AccountType: "liability", SubType: "current_liability", SortOrder: 219},
			}},
			{Code: "2200", Name: "非流動負債", NameEN: "Non-Current Liabilities", AccountType: "liability", SubType: "long_term_liability", SortOrder: 220, Children: []DefaultAccount{
				{Code: "2210", Name: "長期借款", NameEN: "Long-term Loans", AccountType: "liability", SubType: "long_term_liability", SortOrder: 221},
				{Code: "2290", Name: "其他非流動負債", NameEN: "Other Non-Current Liabilities", AccountType: "liability", SubType: "long_term_liability", SortOrder: 229},
			}},
		}},
		// ===== 權益 (Equity) 3xxx =====
		{Code: "3000", Name: "業主權益", NameEN: "Equity", AccountType: "equity", SubType: "", SortOrder: 300, Children: []DefaultAccount{
			{Code: "3100", Name: "股本/資本額", NameEN: "Capital Stock", AccountType: "equity", SubType: "", SortOrder: 310},
			{Code: "3200", Name: "保留盈餘", NameEN: "Retained Earnings", AccountType: "equity", SubType: "", SortOrder: 320},
			{Code: "3300", Name: "本期損益", NameEN: "Current Period Earnings", AccountType: "equity", SubType: "", SortOrder: 330},
		}},
		// ===== 收入 (Revenue) 4xxx =====
		{Code: "4000", Name: "收入", NameEN: "Revenue", AccountType: "revenue", SubType: "", SortOrder: 400, Children: []DefaultAccount{
			{Code: "4100", Name: "營業收入", NameEN: "Operating Revenue", AccountType: "revenue", SubType: "operating_revenue", SortOrder: 410, Children: []DefaultAccount{
				{Code: "4110", Name: "銷貨收入", NameEN: "Sales Revenue", AccountType: "revenue", SubType: "operating_revenue", SortOrder: 411},
				{Code: "4120", Name: "服務收入", NameEN: "Service Revenue", AccountType: "revenue", SubType: "operating_revenue", SortOrder: 412},
				{Code: "4130", Name: "佣金收入", NameEN: "Commission Income", AccountType: "revenue", SubType: "operating_revenue", SortOrder: 413},
				{Code: "4190", Name: "其他營業收入", NameEN: "Other Operating Revenue", AccountType: "revenue", SubType: "operating_revenue", SortOrder: 419},
			}},
			{Code: "4200", Name: "營業外收入", NameEN: "Non-Operating Income", AccountType: "revenue", SubType: "other_income", SortOrder: 420, Children: []DefaultAccount{
				{Code: "4210", Name: "利息收入", NameEN: "Interest Income", AccountType: "revenue", SubType: "other_income", SortOrder: 421},
				{Code: "4220", Name: "處分資產利益", NameEN: "Gain on Disposal of Assets", AccountType: "revenue", SubType: "other_income", SortOrder: 422},
				{Code: "4290", Name: "其他營業外收入", NameEN: "Other Non-Operating Income", AccountType: "revenue", SubType: "other_income", SortOrder: 429},
			}},
		}},
		// ===== 費用 (Expenses) 5xxx-6xxx =====
		{Code: "5000", Name: "銷貨成本", NameEN: "Cost of Goods Sold", AccountType: "expense", SubType: "cogs", SortOrder: 500, Children: []DefaultAccount{
			{Code: "5100", Name: "進貨成本", NameEN: "Purchase Cost", AccountType: "expense", SubType: "cogs", SortOrder: 510},
			{Code: "5200", Name: "直接人工", NameEN: "Direct Labor", AccountType: "expense", SubType: "cogs", SortOrder: 520},
			{Code: "5300", Name: "製造費用", NameEN: "Manufacturing Overhead", AccountType: "expense", SubType: "cogs", SortOrder: 530},
		}},
		{Code: "6000", Name: "營業費用", NameEN: "Operating Expenses", AccountType: "expense", SubType: "operating_expense", SortOrder: 600, Children: []DefaultAccount{
			{Code: "6100", Name: "薪資費用", NameEN: "Salary Expense", AccountType: "expense", SubType: "operating_expense", SortOrder: 610},
			{Code: "6200", Name: "租金費用", NameEN: "Rent Expense", AccountType: "expense", SubType: "operating_expense", SortOrder: 620},
			{Code: "6300", Name: "水電費", NameEN: "Utilities Expense", AccountType: "expense", SubType: "operating_expense", SortOrder: 630},
			{Code: "6400", Name: "辦公用品", NameEN: "Office Supplies", AccountType: "expense", SubType: "operating_expense", SortOrder: 640},
			{Code: "6500", Name: "折舊費用", NameEN: "Depreciation Expense", AccountType: "expense", SubType: "operating_expense", SortOrder: 650},
			{Code: "6600", Name: "佣金費用", NameEN: "Commission Expense", AccountType: "expense", SubType: "operating_expense", SortOrder: 660},
			{Code: "6700", Name: "行銷費用", NameEN: "Marketing Expense", AccountType: "expense", SubType: "operating_expense", SortOrder: 670},
			{Code: "6800", Name: "保險費用", NameEN: "Insurance Expense", AccountType: "expense", SubType: "operating_expense", SortOrder: 680},
			{Code: "6900", Name: "其他營業費用", NameEN: "Other Operating Expenses", AccountType: "expense", SubType: "operating_expense", SortOrder: 690},
		}},
		// ===== 營業外費用 7xxx =====
		{Code: "7000", Name: "營業外費用", NameEN: "Non-Operating Expenses", AccountType: "expense", SubType: "other_expense", SortOrder: 700, Children: []DefaultAccount{
			{Code: "7100", Name: "利息費用", NameEN: "Interest Expense", AccountType: "expense", SubType: "other_expense", SortOrder: 710},
			{Code: "7200", Name: "處分資產損失", NameEN: "Loss on Disposal of Assets", AccountType: "expense", SubType: "other_expense", SortOrder: 720},
			{Code: "7900", Name: "其他營業外費用", NameEN: "Other Non-Operating Expenses", AccountType: "expense", SubType: "other_expense", SortOrder: 790},
		}},
		// ===== 稅費 8xxx =====
		{Code: "8000", Name: "所得稅費用", NameEN: "Income Tax Expense", AccountType: "expense", SubType: "tax_expense", SortOrder: 800},
	}
}
