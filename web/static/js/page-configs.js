// 頁面配置系統 - 定義所有模塊的列表和表單配置

// 全世界國家區碼硬編碼列表（用於 phone-country-codes 表單）
const COUNTRY_PHONE_CODES = {
    '+1': 'United States / Canada',
    '+7': 'Russia / Kazakhstan',
    '+20': 'Egypt',
    '+27': 'South Africa',
    '+30': 'Greece',
    '+31': 'Netherlands',
    '+32': 'Belgium',
    '+33': 'France',
    '+34': 'Spain',
    '+36': 'Hungary',
    '+39': 'Italy',
    '+40': 'Romania',
    '+41': 'Switzerland',
    '+43': 'Austria',
    '+44': 'United Kingdom',
    '+45': 'Denmark',
    '+46': 'Sweden',
    '+47': 'Norway',
    '+48': 'Poland',
    '+49': 'Germany',
    '+51': 'Peru',
    '+52': 'Mexico',
    '+53': 'Cuba',
    '+54': 'Argentina',
    '+55': 'Brazil',
    '+56': 'Chile',
    '+57': 'Colombia',
    '+58': 'Venezuela',
    '+60': 'Malaysia',
    '+61': 'Australia',
    '+62': 'Indonesia',
    '+63': 'Philippines',
    '+64': 'New Zealand',
    '+65': 'Singapore',
    '+66': 'Thailand',
    '+81': 'Japan',
    '+82': 'South Korea',
    '+84': 'Vietnam',
    '+86': 'China',
    '+90': 'Turkey',
    '+91': 'India',
    '+92': 'Pakistan',
    '+93': 'Afghanistan',
    '+94': 'Sri Lanka',
    '+95': 'Myanmar',
    '+98': 'Iran',
    '+212': 'Morocco',
    '+213': 'Algeria',
    '+216': 'Tunisia',
    '+218': 'Libya',
    '+220': 'Gambia',
    '+221': 'Senegal',
    '+222': 'Mauritania',
    '+223': 'Mali',
    '+224': 'Guinea',
    '+225': 'Ivory Coast',
    '+226': 'Burkina Faso',
    '+227': 'Niger',
    '+228': 'Togo',
    '+229': 'Benin',
    '+230': 'Mauritius',
    '+231': 'Liberia',
    '+232': 'Sierra Leone',
    '+233': 'Ghana',
    '+234': 'Nigeria',
    '+235': 'Chad',
    '+236': 'Central African Republic',
    '+237': 'Cameroon',
    '+238': 'Cape Verde',
    '+239': 'São Tomé and Príncipe',
    '+240': 'Equatorial Guinea',
    '+241': 'Gabon',
    '+242': 'Republic of the Congo',
    '+243': 'Democratic Republic of the Congo',
    '+244': 'Angola',
    '+245': 'Guinea-Bissau',
    '+246': 'British Indian Ocean Territory',
    '+248': 'Seychelles',
    '+249': 'Sudan',
    '+250': 'Rwanda',
    '+251': 'Ethiopia',
    '+252': 'Somalia',
    '+253': 'Djibouti',
    '+254': 'Kenya',
    '+255': 'Tanzania',
    '+256': 'Uganda',
    '+257': 'Burundi',
    '+258': 'Mozambique',
    '+260': 'Zambia',
    '+261': 'Madagascar',
    '+262': 'Réunion / Mayotte',
    '+263': 'Zimbabwe',
    '+264': 'Namibia',
    '+265': 'Malawi',
    '+266': 'Lesotho',
    '+267': 'Botswana',
    '+268': 'Swaziland',
    '+269': 'Comoros',
    '+290': 'Saint Helena',
    '+291': 'Eritrea',
    '+297': 'Aruba',
    '+298': 'Faroe Islands',
    '+299': 'Greenland',
    '+350': 'Gibraltar',
    '+351': 'Portugal',
    '+352': 'Luxembourg',
    '+353': 'Ireland',
    '+354': 'Iceland',
    '+355': 'Albania',
    '+356': 'Malta',
    '+357': 'Cyprus',
    '+358': 'Finland',
    '+359': 'Bulgaria',
    '+370': 'Lithuania',
    '+371': 'Latvia',
    '+372': 'Estonia',
    '+373': 'Moldova',
    '+374': 'Armenia',
    '+375': 'Belarus',
    '+376': 'Andorra',
    '+377': 'Monaco',
    '+378': 'San Marino',
    '+380': 'Ukraine',
    '+381': 'Serbia',
    '+382': 'Montenegro',
    '+383': 'Kosovo',
    '+385': 'Croatia',
    '+386': 'Slovenia',
    '+387': 'Bosnia and Herzegovina',
    '+389': 'North Macedonia',
    '+420': 'Czech Republic',
    '+421': 'Slovakia',
    '+423': 'Liechtenstein',
    '+500': 'Falkland Islands',
    '+501': 'Belize',
    '+502': 'Guatemala',
    '+503': 'El Salvador',
    '+504': 'Honduras',
    '+505': 'Nicaragua',
    '+506': 'Costa Rica',
    '+507': 'Panama',
    '+508': 'Saint Pierre and Miquelon',
    '+509': 'Haiti',
    '+590': 'Guadeloupe',
    '+591': 'Bolivia',
    '+592': 'Guyana',
    '+593': 'Ecuador',
    '+594': 'French Guiana',
    '+595': 'Paraguay',
    '+596': 'Martinique',
    '+597': 'Suriname',
    '+598': 'Uruguay',
    '+599': 'Netherlands Antilles',
    '+670': 'East Timor',
    '+672': 'Antarctica / Australian External Territories',
    '+673': 'Brunei',
    '+674': 'Nauru',
    '+675': 'Papua New Guinea',
    '+676': 'Tonga',
    '+677': 'Solomon Islands',
    '+678': 'Vanuatu',
    '+679': 'Fiji',
    '+680': 'Palau',
    '+681': 'Wallis and Futuna',
    '+682': 'Cook Islands',
    '+683': 'Niue',
    '+685': 'Samoa',
    '+686': 'Kiribati',
    '+687': 'New Caledonia',
    '+688': 'Tuvalu',
    '+689': 'French Polynesia',
    '+690': 'Tokelau',
    '+691': 'Micronesia',
    '+692': 'Marshall Islands',
    '+850': 'North Korea',
    '+852': 'Hong Kong',
    '+853': 'Macau',
    '+855': 'Cambodia',
    '+856': 'Laos',
    '+880': 'Bangladesh',
    '+886': 'Taiwan',
    '+960': 'Maldives',
    '+961': 'Lebanon',
    '+962': 'Jordan',
    '+963': 'Syria',
    '+964': 'Iraq',
    '+965': 'Kuwait',
    '+966': 'Saudi Arabia',
    '+967': 'Yemen',
    '+968': 'Oman',
    '+970': 'Palestine',
    '+971': 'United Arab Emirates',
    '+972': 'Israel',
    '+973': 'Bahrain',
    '+974': 'Qatar',
    '+975': 'Bhutan',
    '+976': 'Mongolia',
    '+977': 'Nepal',
    '+992': 'Tajikistan',
    '+993': 'Turkmenistan',
    '+994': 'Azerbaijan',
    '+995': 'Georgia',
    '+996': 'Kyrgyzstan',
    '+998': 'Uzbekistan',
    '+1242': 'Bahamas',
    '+1246': 'Barbados',
    '+1264': 'Anguilla',
    '+1268': 'Antigua and Barbuda',
    '+1284': 'British Virgin Islands',
    '+1340': 'U.S. Virgin Islands',
    '+1345': 'Cayman Islands',
    '+1441': 'Bermuda',
    '+1473': 'Grenada',
    '+1649': 'Turks and Caicos Islands',
    '+1664': 'Montserrat',
    '+1670': 'Northern Mariana Islands',
    '+1671': 'Guam',
    '+1684': 'American Samoa',
    '+1721': 'Sint Maarten',
    '+1758': 'Saint Lucia',
    '+1767': 'Dominica',
    '+1784': 'Saint Vincent and the Grenadines',
    '+1787': 'Puerto Rico',
    '+1809': 'Dominican Republic',
    '+1829': 'Dominican Republic',
    '+1849': 'Dominican Republic',
    '+1868': 'Trinidad and Tobago',
    '+1869': 'Saint Kitts and Nevis',
    '+1876': 'Jamaica',
    '+1939': 'Puerto Rico'
};

const PageConfigs = {
    // 客戶管理
    customers: {
        title: '客戶管理',
        icon: 'bi-people',
        apiPath: '/customers',
        listPath: '/customers',
        editPath: '/customers',
        enableDeleteActions: true,
        exportEnabled: true,
        labelFilter: { apiPath: '/customer-labels', paramKey: 'label_id', defaultShow: 4 },
        columns: [
            { key: 'code', label: '編號', type: 'text' },
            { key: 'name', label: '名稱', type: 'text-with-avatar' },
            { key: 'labels', label: '標籤', type: 'labels' },
            { key: 'email', label: '郵箱', type: 'email' },
            { key: 'phone', label: '電話', type: 'text' },
            { key: 'member_level.name', label: '會員等級', type: 'relation' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholderKey: 'customersPage.searchPlaceholder',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ]},
            { key: 'member_level_id', label: '會員等級', type: 'select2', relationApi: '/member-levels', relationLabel: 'name', required: false }
        ],
        formFields: [
            { key: 'code', label: '編號', type: 'text', required: false, readonly: true },
            { key: 'profile_pic', label: '頭像', type: 'profile-image', required: false },
            { key: 'name', label: '名稱', type: 'text', required: true, maxlength: 50 },
            { key: 'last_name', label: '姓氏（可選）', type: 'text', required: false, placeholder: '例如：張、Smith' },
            { key: 'email', label: '郵箱', type: 'email', required: false },
            { key: 'password', label: '密碼（網店登入用）', type: 'password', required: false, minlength: 6, maxlength: 20, placeholder: '留空則不設置密碼' },
            { key: 'phone_country_code', label: '電話區號', type: 'select2', relationApi: '/api/v1/phone-country-codes', relationLabel: 'code', relationValueKey: 'code', required: false, defaultValue: '+852' },
            { key: 'phone', label: '電話', type: 'text', required: false },
            { key: 'birth_date', label: '出生日期', type: 'date', required: false },
            { key: 'gender', label: '性別', type: 'button-group', required: false, defaultValue: 'unknown', options: [
                { value: 'male', label: '男性', labelKey: 'fields.genderMale' },
                { value: 'female', label: '女性', labelKey: 'fields.genderFemale' },
                { value: 'unknown', label: '未知', labelKey: 'fields.genderUnknown' }
            ]},
            { key: 'my_referral_code', label: '我的推薦碼', type: 'text', required: false, readonly: true, placeholder: '自動生成' },
            { key: 'referral_code', label: '介紹人代碼', type: 'text', required: false, placeholder: '輸入介紹人代碼' },
            { key: 'member_level_id', label: '會員等級', type: 'select2', relationApi: '/member-levels', relationLabel: 'name', required: false },
            { key: 'label_ids', label: '客戶標籤', type: 'select2-multi', relationApi: '/customer-labels', relationLabel: 'name', required: false, fullWidth: true, preferLabel: true },
            { key: 'status', label: '狀態', type: 'select', required: false, defaultValue: 'active', options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ]}
        ]
    },

    // 客戶標籤
    'customer-labels': {
        title: '客戶標籤',
        icon: 'bi-tags',
        apiPath: '/customer-labels',
        listPath: '/customer-labels',
        editPath: '/customer-labels',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'color', label: '顏色', type: 'text' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索客戶標籤...',
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true, fullWidth: true },
            { key: 'color', label: '顏色', type: 'color', required: false, defaultValue: '#007bff', fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, defaultValue: 'active', options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '停用', labelKey: 'common.inactive' }
            ], fullWidth: true }
        ]
    },

    // 打卡報告（統計）
    'attendance-reports': {
        title: 'menu.attendanceReports',
        icon: 'bi-file-earmark-text',
        apiPath: '/attendance-reports',
        listPath: '/attendance-reports',
        editPath: '/attendance-reports',
        exportEnabled: true,
        formFields: [],
        showActions: false,
        showDraftButton: false,
        columns: [
            { key: 'user_name', label: 'fields.employee', type: 'text' },
            { key: 'start_date', label: 'fields.startDate', type: 'date' },
            { key: 'end_date', label: 'fields.endDate', type: 'date' },
            { key: 'total_days', label: 'attendanceReports.totalDays', type: 'number' },
            { key: 'normal_days', label: 'attendanceReports.normalDays', type: 'number' },
            { key: 'late_days', label: 'attendanceReports.lateDays', type: 'number' },
            { key: 'early_leave_days', label: 'attendanceReports.earlyLeaveDays', type: 'number' },
            { key: 'absent_days', label: 'attendanceReports.absentDays', type: 'number' },
            { key: 'total_work_minutes', label: 'attendanceReports.totalWorkMinutes', type: 'number' },
            { key: 'total_ot_minutes', label: 'attendanceReports.totalOtMinutes', type: 'number' }
        ],
        searchPlaceholderKey: 'attendanceReports.searchPlaceholder',
        filters: [
            { key: 'start_date', label: 'fields.startDate', type: 'date', defaultValue: (function(){ const d=new Date(); return new Date(d.getFullYear(), d.getMonth(), 1).toISOString().slice(0,10); })(), fullWidth: false, colWidth: '6' },
            { key: 'end_date', label: 'fields.endDate', type: 'date', defaultValue: (function(){ const d=new Date(); return new Date(d.getFullYear(), d.getMonth()+1, 0).toISOString().slice(0,10); })(), fullWidth: false, colWidth: '6' }
        ]
    },

    // 支出申請
    'expense-requests': {
        title: '支出申請',
        icon: 'bi-question-circle',
        apiPath: '/expense-requests',
        listPath: '/expense-requests',
        editPath: '/expense-requests',
        exportEnabled: true,
        columns: [
            { key: 'title', label: '標題', type: 'text' },
            { key: 'amount', label: '金額', type: 'currency' },
            { key: 'request_date', label: '申請日期', type: 'date' },
            { key: 'status', label: '狀態', type: 'badge', options: { pending: 'warning', approved: 'success', rejected: 'danger' }, labels: { pending: '待批核', approved: '已批核', rejected: '已拒絕' } }
        ],
        searchPlaceholder: '搜索標題...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'pending', label: '待批核' },
                { value: 'approved', label: '已批核' },
                { value: 'rejected', label: '已拒絕' }
            ]}
        ],
        formFields: [
            { key: 'title', label: '標題', type: 'text', required: true },
            { key: 'amount', label: '金額', type: 'number', required: true, step: '0.01' },
            { key: 'request_date', label: '申請日期', type: 'date', required: true },
            { key: 'attachment', label: '附件', type: 'file', required: false, accept: '*/*' },
            { key: 'description', label: '描述', type: 'textarea', required: false }
        ]
    },

    // 供應商管理
    suppliers: {
        title: '供應商管理',
        icon: 'bi-truck',
        apiPath: '/suppliers',
        listPath: '/suppliers',
        editPath: '/suppliers',
        exportEnabled: true,
        columns: [
            { key: 'code', label: '編號', type: 'text' },
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'email', label: '郵箱', type: 'email' },
            { key: 'phone', label: '電話', type: 'text' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索供應商名稱、郵箱或電話...',
        formFields: [
            { key: 'code', label: '編號', type: 'text', required: false, readonly: true, fullWidth: true },
            { key: 'name', label: '名稱', type: 'text', required: true, maxlength: 50 },
            { key: 'last_name', label: '姓氏（可選）', type: 'text', required: false, placeholder: '例如：張、Smith' },
            { key: 'email', label: '郵箱', type: 'email', required: false },
            { key: 'phone_country_code', label: '電話區號', type: 'select2', relationApi: '/api/v1/phone-country-codes', relationLabel: 'code', relationValueKey: 'code', required: false, defaultValue: '+852' },
            { key: 'phone', label: '電話', type: 'text', required: false },
            { key: 'address_country_code', label: '國家', type: 'select2', relationApi: '/api/v1/countries', relationLabel: 'name', relationValueKey: 'code', required: false },
            { key: 'address_region_code', label: '地區', type: 'select2', relationApi: '/api/v1/country-regions', relationLabel: 'name', relationValueKey: 'code', required: false },
            { key: 'address', label: '地址', type: 'textarea', required: false },
            { key: 'status', label: '狀態', type: 'select', required: false, defaultValue: 'active', options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ]}
        ]
    },

    // 產品管理
    products: {
        title: '產品管理',
        icon: 'bi-box',
        apiPath: '/products',
        listPath: '/products',
        editPath: '/products',
        exportEnabled: true,
        labelFilter: { apiPath: '/product-labels', paramKey: 'label_id', defaultShow: 4 },
        columns: [
            { key: 'image_url', label: '圖片', type: 'image', default: '/static/product.jpg' },
            { key: 'code', label: '編號', type: 'text' },
            { key: 'sku', label: 'SKU', type: 'text' },
            { key: 'barcode', label: '條碼', type: 'text' },
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'labels', label: '標籤', type: 'labels' },
            { key: 'product_type.name', label: '分類', type: 'relation' },
            { key: 'price', label: '價格', type: 'currency' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索產品名稱、編號或描述...',
        filters: [
            { key: 'product_type_id', label: '分類', type: 'select2', relationApi: '/product-types', relationLabel: 'name', required: false }
        ],
        formFields: [
            { key: 'code', label: '編號', type: 'text', required: false, readonly: true },
            { key: 'name', label: '名稱', type: 'text', required: true, maxlength: 50 },
            { key: 'image_url', label: '圖片', type: 'file', required: false, accept: 'image/*' },
            { key: 'sku', label: 'SKU', type: 'text', required: false },
            { key: 'barcode', label: '條碼', type: 'text', required: false },
            { key: 'description', label: '描述', type: 'html-editor', required: false, placeholder: '輸入內容...' },
            { key: 'substance_category', label: '實質類別', type: 'text', required: false, placeholder: '例如：實體、數位、服務' },
                        { key: 'product_type_ids', label: '產品類型', type: 'select2-multi', relationApi: '/product-types', relationLabel: 'name', required: false,
                            relationDisplayFormat: (item) => {
                                    if (!item) return '';
                                    if (item.parent && item.parent.name) {
                                            return `${item.parent.name} / ${item.name || item.code || item.id}`;
                                    }
                                    return item.name || item.code || item.id;
                            }
                        },
            { key: 'brand_id', label: '品牌', type: 'select2', relationApi: '/brands', relationLabel: 'name', required: false },
            { key: 'label_ids', label: '產品標籤', type: 'select2-multi', relationApi: '/product-labels', relationLabel: 'name', required: false, fullWidth: true },
            { key: 'price', label: '價格', type: 'number', required: false, step: '0.01' },
            { key: 'cost', label: '成本', type: 'number', required: false, step: '0.01' },
            { key: 'unit', label: '單位', type: 'text', required: false },
            { key: 'is_service_package', label: '服務套票', type: 'select', required: false, options: [
                { value: 'false', label: 'No' },
                { value: 'true', label: 'Yes' }
            ], defaultValue: 'false', onChange: 'toggleServicePackageService' },
            { key: 'service_package_service_id', label: '對應服務', type: 'select2', relationApi: '/services', relationLabel: 'name', required: false, fullWidth: true, dependency: { field: 'is_service_package', value: 'true' } },
            { key: 'is_non_inventory', label: '非庫存類產品', type: 'select', required: false, options: [
                { value: 'false', label: 'No' },
                { value: 'true', label: 'Yes' }
            ], defaultValue: 'false', fullWidth: true, helpText: '如設為 Yes，所有庫存和配送相關功能將不計算此產品', helpTextKey: 'products.helpTexts.is_non_inventory' },
            { key: 'allow_backorder', label: '允許缺貨訂購', type: 'select', required: false, options: [
                { value: 'false', label: 'No' },
                { value: 'true', label: 'Yes' }
            ], defaultValue: 'false', fullWidth: true },
            { key: 'default_warehouse_zone_id', label: '預設倉庫區', labelKey: 'fields.default_warehouse_zone_id', type: 'select2', relationApi: '/warehouse-zones', relationLabel: 'name', required: false, fullWidth: true,
                relationDisplayFormat: (item) => {
                    if (!item) return '';
                    if (item.warehouse && item.warehouse.name) {
                        return `${item.warehouse.name} / ${item.name}`;
                    }
                    return item.name || item.code || item.id;
                }
            },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: 'active', fullWidth: true},
            { key: 'show_on_vmarket', label: '在 VMarket 顯示', type: 'checkbox', required: false, defaultValue: false, fullWidth: true, helpText: '加入 VMarket 後才會對外顯示', helpTextKey: 'products.helpTexts.show_on_vmarket' },
            { key: 'category', label: 'VMarket 分類', type: 'select2', required: false, fullWidth: true,
                dependency: { field: 'show_on_vmarket', value: 'true' },
                defaultValue: '未分類',
                options: [
                    { value: '未分類', label: '未分類' },
                    { value: '食品與飲料', label: '食品與飲料' },
                    { value: '生鮮蔬果', label: '生鮮蔬果' },
                    { value: '零食與甜點', label: '零食與甜點' },
                    { value: '保健食品', label: '保健食品' },
                    { value: '美妝與護膚', label: '美妝與護膚' },
                    { value: '個人護理', label: '個人護理' },
                    { value: '服飾與配件', label: '服飾與配件' },
                    { value: '鞋類與包包', label: '鞋類與包包' },
                    { value: '珠寶與飾品', label: '珠寶與飾品' },
                    { value: '3C 電子產品', label: '3C 電子產品' },
                    { value: '電腦與週邊', label: '電腦與週邊' },
                    { value: '手機與平板', label: '手機與平板' },
                    { value: '家電與廚具', label: '家電與廚具' },
                    { value: '家具與居家', label: '家具與居家' },
                    { value: '家飾與燈具', label: '家飾與燈具' },
                    { value: '寵物用品', label: '寵物用品' },
                    { value: '母嬰與兒童', label: '母嬰與兒童' },
                    { value: '玩具與模型', label: '玩具與模型' },
                    { value: '運動與戶外', label: '運動與戶外' },
                    { value: '健身器材', label: '健身器材' },
                    { value: '汽車與機車', label: '汽車與機車' },
                    { value: '圖書與文具', label: '圖書與文具' },
                    { value: '音樂與影視', label: '音樂與影視' },
                    { value: '遊戲與娛樂', label: '遊戲與娛樂' },
                    { value: '辦公用品', label: '辦公用品' },
                    { value: '五金與工具', label: '五金與工具' },
                    { value: '園藝與花卉', label: '園藝與花卉' },
                    { value: '藝術與手工藝', label: '藝術與手工藝' },
                    { value: '旅行與戶外裝備', label: '旅行與戶外裝備' },
                    { value: '禮品與節慶', label: '禮品與節慶' },
                    { value: '數位商品', label: '數位商品' },
                    { value: '軟體與應用', label: '軟體與應用' },
                    { value: '課程與教育', label: '課程與教育' },
                    { value: '專業服務', label: '專業服務' },
                    { value: '其他', label: '其他' }
                ]
            }
        ]
    },

    // 產品稅
    'product-taxes': {
        title: '產品稅',
        icon: 'bi-percent',
        apiPath: '/product-taxes',
        listPath: '/product-taxes',
        editPath: '/product-taxes',
        exportEnabled: true,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'code', label: '代碼', type: 'text' },
            { key: 'tax_mode', label: '模式', type: 'text' },
            { key: 'tax_value', label: '稅值', type: 'number' },
            { key: 'default_include', label: '訂單預設包含', type: 'default-include', options: [
                { value: 'order', label: '' }
            ] },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索產品稅名稱或代碼...',
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true, fullWidth: true },
            { key: 'code', label: '代碼', type: 'text', required: false, fullWidth: true },
            { key: 'tax_mode', label: '計算模式', type: 'select', required: true, defaultValue: 'percent', options: [
                { value: 'percent', label: '百分比 (%)' },
                { value: 'fixed', label: '固定金額' }
            ], fullWidth: true },
            { key: 'tax_value', label: '稅值', type: 'number', required: true, step: '0.0001', defaultValue: 0, fullWidth: true },
            { key: 'default_include', label: '訂單預設包含', type: 'default-include-single', includeValue: 'order', required: false, fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, defaultValue: 'active', options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '停用', labelKey: 'common.inactive' }
            ], fullWidth: true }
        ]
    },

    // 訂單管理
    orders: {
        title: '訂單管理',
        icon: 'bi-cart',
        apiPath: '/orders',
        listPath: '/orders',
        editPath: '/orders',
        exportEnabled: true,
        labelFilter: { apiPath: '/order-labels', paramKey: 'label_id', defaultShow: 4 },
        moreActions: [
            { id: 'importFromExcel', label: '匯入 Excel', icon: 'bi bi-upload' },
            { id: 'downloadImportTemplate', label: '下載匯入 Excel（不含資料）', icon: 'bi bi-download', arg: false },
            { id: 'downloadImportTemplate', label: '下載匯入 Excel（含資料）', icon: 'bi bi-download', arg: true },
            { type: 'divider' }
        ],
        columns: [
            { key: 'order_number', label: '訂單號', type: 'text' },
            { key: 'source_type', label: '來源', type: 'badge', options: { erp: 'secondary', pos: 'info', webstore: 'primary', dining: 'success' } },
            { key: 'customer', label: '客戶', type: 'relation', relationKey: 'name' },
            { key: 'products', label: '產品', type: 'tags' },
            { key: 'labels', label: '標籤', type: 'labels' },
            { key: 'order_date', label: '日期', type: 'date' },
            { key: 'total_amount', label: '金額', type: 'currency' },
            { key: 'status', label: '狀態', type: 'badge', options: {
                draft: 'secondary',
                quotation: 'warning',
                confirmed: 'info',
                processing: 'warning',
                completed: 'success',
                cancelled: 'danger'
            }}
        ],
        searchPlaceholderKey: 'ordersPage.searchPlaceholder',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部狀態', labelKey: 'common.allStatuses' },
                { value: 'quotation', label: '報價單' },
                { value: 'confirmed', label: '已確認' },
                { value: 'processing', label: '處理中' },
                { value: 'completed', label: '已完成' },
                { value: 'cancelled', label: '已取消' }
            ]},
            { key: 'source_type', label: '來源', type: 'select', options: [
                { value: '', label: '全部來源', labelKey: 'common.allSources' },
                { value: 'erp', label: 'ERP' },
                { value: 'pos', label: 'POS' },
                { value: 'webstore', label: '網店' },
                { value: 'dining', label: '點餐' }
            ]}
        ],
        formFields: [], // 訂單使用特殊表單，這裡留空
        showAddButton: true // 顯示新增按鈕
    },

    // 報價單管理
    quotations: {
        title: '報價單管理',
        icon: 'bi-file-earmark-text',
        apiPath: '/quotations',
        listPath: '/quotations',
        editPath: '/quotations',
        exportEnabled: true,
        labelFilter: { apiPath: '/order-labels', paramKey: 'label_id', defaultShow: 4 },
        columns: [
            { key: 'order_number', label: '報價單號', type: 'text' },
            { key: 'customer', label: '客戶', type: 'relation', relationKey: 'name' },
            { key: 'labels', label: '標籤', type: 'labels' },
            { key: 'order_date', label: '日期', type: 'date' },
            { key: 'total_amount', label: '金額', type: 'currency' },
            { key: 'status', label: '狀態', type: 'badge', options: {
                quotation: 'warning'
            }, labels: {
                quotation: '報價單'
            }}
        ],
        searchPlaceholder: '搜索報價單號或客戶名稱...',
        filters: [],
        formFields: [], // 報價單使用特殊表單，這裡留空
        showAddButton: true // 顯示新增按鈕
    },

    // 發票管理
    invoices: {
        title: '發票管理',
        icon: 'bi-receipt',
        apiPath: '/invoices',
        listPath: '/invoices',
        editPath: '/invoices',
        exportEnabled: true,
        columns: [
            { key: 'invoice_number', label: '發票號', type: 'text' },
            { key: 'customer', label: '客戶', type: 'relation', relationKey: 'name' },
            { key: 'invoice_date', label: '日期', type: 'date' },
            { key: 'total_amount', label: '總金額', type: 'currency' },
            { key: 'paid_amount', label: '已付', type: 'currency' },
            { key: 'status', label: '狀態', type: 'badge', options: {
                draft: 'secondary',
                sent: 'info',
                paid: 'success',
                overdue: 'danger',
                cancelled: 'secondary'
            }, labels: {
                draft: '草稿',
                sent: '已發送',
                paid: '已付款',
                overdue: '逾期',
                cancelled: '已取消'
            }}
        ],
        searchPlaceholder: '搜索發票號或客戶名稱...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部狀態' },
                { value: 'draft', label: '草稿' },
                { value: 'sent', label: '已發送' },
                { value: 'paid', label: '已付款' },
                { value: 'overdue', label: '逾期' },
                { value: 'cancelled', label: '已取消' }
            ]}
        ],
        formFields: [], // 發票使用特殊表單
        showAddButton: true // 顯示新增按鈕
    },

    // 公司管理
    'companies': {
        title: '公司管理',
        icon: 'bi-building',
        apiPath: '/companies',
        listPath: '/companies',
        editPath: '/companies',
        exportEnabled: true,
        columns: [
            { key: 'code', label: '編號', type: 'text' },
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        formFields: [
            { key: 'code', label: '編號', type: 'text', required: false },
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'address', label: '地址', type: 'textarea', required: false },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ]}
        ]
    },

    // 部門管理
    'departments': {
        title: '部門管理',
        icon: 'bi-diagram-3',
        apiPath: '/departments',
        listPath: '/departments',
        editPath: '/departments',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'description', label: '描述', type: 'text' }
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'description', label: '描述', type: 'textarea', required: false }
        ]
    },

    // 角色管理（原級別管理）
    'roles': {
        title: '角色管理',
        icon: 'bi-person-badge',
        apiPath: '/roles',
        listPath: '/roles',
        editPath: '/roles',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'description', label: '描述', type: 'text' },
            { key: 'status', label: '狀態', type: 'badge', options: [
                { value: 'active', label: '活躍', labelKey: 'common.active', color: 'success' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive', color: 'secondary' }
            ]}
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true, fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ], fullWidth: true },
            { key: 'description', label: '描述', type: 'textarea', required: false },
            { key: 'permissions', label: '菜單權限', type: 'checkbox-group', required: false, fullWidth: true, sessions: [
                { key: 'dashboard', label: '核心業務', options: [
                    { value: 'dashboard', label: '儀表板' },
                    { value: 'business-goals', label: '業務目標' },
                    { value: 'pos', label: 'POS 收銀台' }
                ]},
                { key: 'customerManagement', label: '客戶管理', options: [
                    { value: 'customers', label: '客戶' },
                    { value: 'customer-labels', label: '客戶標籤' },
                    { value: 'member-levels', label: '會員等級' },
                    { value: 'points', label: '積分' },
                    { value: 'referrals', label: '介紹記錄' },
                    { value: 'coupons', label: '優惠券' },
                    { value: 'point-settings', label: '積分設置' },
                    { value: 'points-history', label: '積分記錄' }
                ]},
                { key: 'productManagement', label: '產品管理', options: [
                    { value: 'products', label: '產品' },
                    { value: 'product-labels', label: '產品標籤' },
                    { value: 'product-types', label: '產品類型' },
                    { value: 'product-attributes', label: '產品屬性' },
                    { value: 'brands', label: '品牌' },
                    { value: 'warehouses', label: '倉庫管理' },
                    { value: 'inventory-adjustments', label: '庫存調整' },
                    { value: 'inventory-counts', label: '庫存盤點' },
                    { value: 'low-stock', label: '低庫存預警' }
                ]},
                { key: 'orderManagement', label: '訂單管理', options: [
                    { value: 'orders', label: '訂單' },
                    { value: 'order-labels', label: '訂單標籤' },
                    { value: 'order-reports', label: '訂單報表' },
                    { value: 'payment-methods', label: '付款方式' },
                    { value: 'shipping-methods', label: '運送方式' },
                    { value: 'logistics-companies', label: '物流公司' }
                ]},
                { key: 'serviceManagement', label: '服務管理', options: [
                    { value: 'service-types', label: '服務種類' },
                    { value: 'services', label: '服務' },
                    { value: 'appointments', label: '預約' },
                    { value: 'service-orders', label: '服務單' },
                    { value: 'service-order-labels', label: '服務標籤' },
                    { value: 'service-staffs', label: '服務員' },
                    { value: 'rooms', label: '房間' },
                    { value: 'equipments', label: '設備' }
                ]},
                { key: 'personalTools', label: '個人工具', options: [
                    { value: 'calendars', label: '日曆' },
                    { value: 'reminders', label: '提示' },
                    { value: 'messages', label: '訊息' },
                    { value: 'notes', label: '備忘' },
                    { value: 'personal-data', label: '帳戶設置' }
                ]},
                { key: 'otherFeatures', label: '宣傳', options: [
                    { value: 'promotions', label: '推廣發送' },
                    { value: 'google-ads', label: 'Google 廣告' }
                ]},
                { key: 'accounting', label: '會計', options: [
                    { value: 'accounting', label: '會計總覽' },
                    { value: 'billing', label: '訂閱' },
                    { value: 'hardware-purchase', label: '訂閱硬件' },
                    { value: 'incomes', label: '收入' },
                    { value: 'expenses', label: '支出' },
                    { value: 'expense-requests', label: '支出申請' },
                    { value: 'purchase-orders', label: '採購' },
                    { value: 'purchase-order-labels', label: '採購標籤' },
                    { value: 'suppliers', label: '供應商' },
                    { value: 'bank-accounts', label: '銀行賬戶' }
                ]},
                { key: 'hrManagement', label: 'HR 管理', options: [
                    { value: 'attendance-clock', label: '打卡' },
                    { value: 'attendances', label: '打卡記錄' },
                    { value: 'leave-requests', label: '請假申請' },
                    { value: 'users', label: '員工' },
                    { value: 'shifts', label: '工作時段' },
                    { value: 'payrolls', label: '薪資記錄' }
                ]},
                { key: 'systemSettings', label: '系統設定', options: [
                    { value: 'document-settings', label: '文件設定' },
                    { value: 'document-auto-settings', label: '單據設定' },
                    { value: 'enterprises', label: '企業設置' },
                    { value: 'departments', label: '部門' },
                    { value: 'roles', label: '角色' },
                    { value: 'field-settings', label: '欄位設定' },
                    { value: 'regions', label: '地區' },
                    { value: 'currencies', label: '貨幣' }
                ]}
            ]}
        ]
    },

    // 地區管理
    'regions': {
        title: '地區管理',
        icon: 'bi-geo-alt',
        apiPath: '/regions',
        listPath: '/regions',
        editPath: '/regions',
        exportEnabled: false,
        columns: [
            { key: 'code', label: '編碼', type: 'text' },
            { key: 'name', label: '名稱', type: 'text' }
        ],
        formFields: [
            { key: 'code', label: '編碼', type: 'text', required: false },
            { key: 'name', label: '名稱', type: 'text', required: true }
        ]
    },

    // 貨幣管理
    'currencies': {
        title: '貨幣管理',
        icon: 'bi-currency-exchange',
        apiPath: '/currencies',
        listPath: '/currencies',
        editPath: '/currencies',
        exportEnabled: false,
        columns: [
            { key: 'code', label: '代碼', type: 'text' },
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'symbol', label: '符號', type: 'text' },
            { key: 'exchange_rate', label: '匯率', type: 'number', format: (value) => value ? parseFloat(value).toFixed(6) : '1.000000' },
            { key: 'is_default', label: '系統預設', type: 'badge', options: { true: 'success', false: 'secondary' } }
        ],
        formFields: [
            { key: 'code', label: '代碼', type: 'text', required: true, placeholder: '例如：HKD, USD' },
            { key: 'name', label: '名稱', type: 'text', required: true, placeholder: '例如：港幣, 美元' },
            { key: 'symbol', label: '符號', type: 'text', required: false, placeholder: '例如：$' },
            { key: 'exchange_rate', label: '匯率', type: 'number', required: false, step: '0.000001', placeholder: '相對基礎貨幣的匯率', defaultValue: 1.0 },
            { key: 'is_default', label: '設為系統預設', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]}
        ]
    },

    // 日曆管理
    'calendars': {
        title: '日曆管理',
        icon: 'bi-calendar',
        apiPath: '/calendars',
        listPath: '/calendars',
        editPath: '/calendars',
        exportEnabled: false,
        columns: [
            { key: 'title', label: '標題', type: 'text' },
            { key: 'start_time', label: '開始時間', type: 'datetime' },
            { key: 'end_time', label: '結束時間', type: 'datetime' }
        ],
        formFields: [
            { key: 'title', label: '標題', type: 'text', required: true },
            { key: 'start_time', label: '開始時間', type: 'datetime-local', required: true },
            { key: 'end_time', label: '結束時間', type: 'datetime-local', required: false },
            { key: 'description', label: '描述', type: 'textarea', required: false },
            { key: 'all_day', label: '全天事件', type: 'select', required: false, options: [
                { value: 'no', label: 'No' },
                { value: 'yes', label: 'Yes' }
            ], defaultValue: 'no' },
            { key: 'event_type', label: '事件類型', type: 'select', required: false, options: [
                { value: '', label: '請選擇' },
                { value: 'meeting', label: '會議' },
                { value: 'deadline', label: '截止日期' },
                { value: 'reminder', label: '提示' },
                { value: 'holiday', label: '假期' },
                { value: 'other', label: '其他' }
            ]}
        ]
    },

    // 提示管理
    'reminders': {
        title: '提示管理',
        icon: 'bi-bell',
        apiPath: '/reminders',
        listPath: '/reminders',
        editPath: '/reminders',
        exportEnabled: false,
        columns: [
            { key: 'title', label: '標題', type: 'text' },
            { key: 'remind_at', label: '提示時間', type: 'date' },
            { key: 'status', label: '狀態', type: 'badge', options: { pending: 'warning', done: 'success' } }
        ],
        formFields: [
            { key: 'title', label: '標題', type: 'text', required: true, fullWidth: true },
            { key: 'remind_at', label: '提示時間', type: 'datetime-local', required: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'pending', label: '待辦' },
                { value: 'done', label: '完成' }
            ]},
            { key: 'notes', label: '內容', type: 'html-editor', required: false }
        ]
    },

    // 訊息管理
    'messages': {
        title: '訊息管理',
        icon: 'bi-envelope',
        apiPath: '/messages',
        listPath: '/messages',
        editPath: '/messages',
        exportEnabled: false,
        columns: [
            { key: 'title', label: '標題', type: 'text' },
            { key: 'type', label: '類型', type: 'text' },
            { key: 'status', label: '狀態', type: 'badge', options: { unread: 'info', read: 'secondary' } }
        ],
        formFields: [
            { key: 'title', label: '標題', type: 'text', required: true },
            { key: 'type', label: '類型', type: 'text', required: false },
            { key: 'content', label: '內容', type: 'textarea', required: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'unread', label: '未讀' },
                { value: 'read', label: '已讀' }
            ]}
        ]
    },

    // 備忘管理
    'notes': {
        title: '備忘管理',
        icon: 'bi-sticky',
        apiPath: '/notes',
        listPath: '/notes',
        editPath: '/notes',
        exportEnabled: false,
        columns: [
            { key: 'title', label: '標題', type: 'text' },
            { key: 'updated_at', label: '更新時間', type: 'date' }
        ],
        formFields: [
            { key: 'title', label: '標題', type: 'text', required: true, fullWidth: true },
            { key: 'content', label: '內容', type: 'html-editor', required: true }
        ]
    },

    // 會員等級
    'member-levels': {
        title: '會員等級管理',
        icon: 'bi-star',
        apiPath: '/member-levels',
        listPath: '/member-levels',
        editPath: '/member-levels',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'level_order', label: '順序', type: 'number' },
            { key: 'discount_rate', label: '折扣率 (%)', type: 'number' },
            { key: 'is_default', label: '系統預設', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } },
            { key: 'auto_upgrade', label: '自動會員升級', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } }
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'code', label: '代碼', type: 'text', required: false },
            { key: 'level_order', label: '順序', type: 'number', required: true, step: '1', min: '0' },
            { key: 'min_points', label: '最低積分', type: 'number', required: false, step: '1', min: '0' },
            { key: 'min_purchase_amount', label: '最低購物金額', type: 'number', required: false, step: '0.01', min: '0' },
            { key: 'discount_rate', label: '折扣率 (%)', type: 'number', required: false, step: '0.01', min: '0', max: '100' },
            { key: 'is_default', label: '設為系統預設會員等級', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'auto_upgrade', label: '自動會員升級', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'description', label: '福利', type: 'textarea', required: false, rows: 3 }
        ]
    },

    // 積分
    'points': {
        title: '積分管理',
        icon: 'bi-trophy',
        apiPath: '/points',
        listPath: '/points',
        editPath: '/points',
        exportEnabled: false,
        columns: [
            { key: 'customer.name', label: '客戶', type: 'relation' },
            { key: 'points', label: '積分', type: 'number' },
            { key: 'points_type', label: '類型', type: 'badge', 
                options: { earned: 'success', redeemed: 'warning', expired: 'secondary', adjusted: 'info' },
                labels: { earned: '獲得', redeemed: '消費', expired: '過期', adjusted: '調整' } },
            { key: 'description', label: '說明', type: 'text' },
            { key: 'created_at', label: '日期', type: 'date' }
        ],
        formFields: [
            { key: 'customer_id', label: '客戶', type: 'select2', relationApi: '/customers', relationValueKey: 'id', relationLabelKey: 'name', required: true },
            { key: 'points', label: '積分', type: 'number', required: true },
            { key: 'points_type', label: '類型', type: 'select', required: true, options: [
                { value: 'earned', label: '獲得' },
                { value: 'redeemed', label: '兌換' }
            ]},
            { key: 'description', label: '說明', type: 'textarea', required: false }
        ]
    },

    // 積分記錄
    'points-history': {
        title: '積分記錄',
        icon: 'bi-clock-history',
        apiPath: '/points-history',
        listPath: '/points-history',
        editPath: '/points-history',
        exportEnabled: false,
        columns: [
            { key: 'customer.name', label: '客戶', type: 'relation' },
            { key: 'points', label: '積分', type: 'number' },
            { key: 'points_type', label: '類型', type: 'badge', 
                options: { earned: 'success', redeemed: 'warning', expired: 'secondary', adjusted: 'info' },
                labels: { earned: '獲得', redeemed: '消費', expired: '過期', adjusted: '調整' } },
            { key: 'description', label: '說明', type: 'text' },
            { key: 'created_at', label: '日期', type: 'date' }
        ],
        searchPlaceholder: '搜索客戶名稱...',
        formFields: [] // 只讀列表，不需要表單
    },

    // 介紹記錄
    'referrals': {
        title: '介紹記錄',
        icon: 'bi-person-check',
        apiPath: '/referrals',
        listPath: '/referrals',
        editPath: '/referrals',
        exportEnabled: true,
        showDraftButton: false, // 不顯示草稿按鈕
        columns: [
            { key: 'customer_name', label: '客戶', type: 'text' },
            { key: 'referral_code', label: '介紹人代碼', type: 'text' },
            { key: 'referrer_name', label: '介紹人', type: 'text' },
            { key: 'created_at', label: '建立時間', type: 'datetime' }
        ],
        searchPlaceholderKey: 'referrals.searchPlaceholder',
        filters: [],
        formFields: [] // 只讀列表，不需要表單
    },

    // 優惠券
    'coupons': {
        title: '優惠券管理',
        icon: 'bi-ticket-perforated',
        apiPath: '/coupons',
        listPath: '/coupons',
        editPath: '/coupons',
        exportEnabled: true,
        columns: [
            { key: 'code', label: '代碼', type: 'text' },
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'coupon_type', label: '類型', type: 'badge', options: { percentage: 'info', fixed_amount: 'primary', free_shipping: 'success' } },
            { key: 'discount_value', label: '折扣值', type: 'number' },
            { key: 'used_count', label: '已使用', type: 'number' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary', expired: 'secondary' } }
        ],
        formFields: [
            { key: 'code', label: '代碼', type: 'text', required: true },
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'description', label: '描述', type: 'textarea', required: false },
            { key: 'coupon_type', label: '類型', type: 'select', required: true, options: [
                { value: 'percentage', label: '百分比折扣' },
                { value: 'fixed_amount', label: '固定金額折扣' },
                { value: 'free_shipping', label: '免運費' }
            ]},
            { key: 'discount_value', label: '折扣值', type: 'number', required: true, step: '0.01' },
            { key: 'max_discount', label: '最大折扣金額（僅百分比）', type: 'number', required: false, step: '0.01' },
            { key: 'min_purchase', label: '最低消費金額', type: 'number', required: false, step: '0.01' },
            { key: 'member_level_id', label: '限制會員等級', type: 'select2', relationApi: '/member-levels', relationLabel: 'name', required: false },
            { key: 'min_product_quantity', label: '最低購物車產品數量', type: 'number', required: false, min: 1 },
            { key: 'min_product_amount', label: '最低購物車產品金額', type: 'number', required: false, step: '0.01' },
            { key: 'valid_from', label: '有效期開始', type: 'datetime-local', required: true },
            { key: 'valid_to', label: '有效期結束', type: 'datetime-local', required: false },
            { key: 'usage_limit', label: '使用次數限制', type: 'number', required: false, min: 1 },
            { key: 'customer_limit', label: '每客戶使用次數限制', type: 'number', required: false, min: 1 },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' },
                { value: 'expired', label: '已過期' }
            ]}
        ]
    },

    // 積分設置
    'point-settings': {
        title: '積分設置',
        icon: 'bi-gear',
        apiPath: '/point-settings',
        listPath: '/point-settings',
        editPath: '/point-settings',
        exportEnabled: false,
        columns: [
            { key: 'earn_points_enabled', label: '消費獲得積分', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } },
            { key: 'points_per_dollar', label: '每消費 1 元獲得積分', type: 'number' },
            { key: 'dollar_per_point', label: '每 1 積分等於金額', type: 'number' },
            { key: 'min_points_to_use', label: '最低可用積分', type: 'number' }
        ],
        formFields: [
            { key: 'earn_points_enabled', label: '消費獲得積分', type: 'select', required: false, fullWidth: true, defaultValue: 'true', options: [
                { value: 'true', label: 'Yes' },
                { value: 'false', label: 'No' }
            ]},
            { key: 'points_per_dollar', label: '每消費 1 元獲得積分', type: 'number', required: false, step: '0.01', defaultValue: 1.0, fullWidth: true },
            { key: 'dollar_per_point', label: '每 1 積分等於金額', type: 'number', required: false, step: '0.0001', defaultValue: 0.01, fullWidth: true },
            { key: 'min_points_to_use', label: '最低可用積分', type: 'number', required: false, defaultValue: 0, fullWidth: true },
            { key: 'max_points_percent', label: '積分最多可抵扣 %（可選）', type: 'number', required: false, step: '0.01', fullWidth: true },
            { key: 'referral_bonus_mode', label: '介紹人獎勵模式', type: 'select', required: false, fullWidth: true, defaultValue: 'fixed', options: [
                { value: 'fixed', label: '固定積分' },
                { value: 'percent', label: '按消費百分比' }
            ]},
            { key: 'referral_bonus_value', label: '介紹人獎勵值', type: 'number', required: false, step: '0.01', fullWidth: true },
            { key: 'referral_count_policy', label: '介紹人計算策略', type: 'select', required: false, fullWidth: true, defaultValue: 'all', options: [
                { value: 'all', label: '所有訂單' },
                { value: 'first_only', label: '只計首單' }
            ]},
            { key: 'status', label: '狀態', type: 'select', required: false, fullWidth: true, defaultValue: 'active', options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '停用', labelKey: 'common.inactive' }
            ]}
        ]
    },

    // 印花設定
    'stamp-settings': {
        title: '印花設定',
        titleKey: 'stampSettingsPage.title',
        icon: 'bi-award',
        apiPath: '/stamp-settings',
        listPath: '/stamp-settings',
        editPath: '/stamp-settings',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '活動名稱', labelKey: 'stampSettingsPage.columns.name', type: 'text' },
            { key: 'product_stamp_enabled', label: '產品印花', labelKey: 'stampSettingsPage.columns.productStampEnabled', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: 'stampSettingsPage.options.yes', false: 'stampSettingsPage.options.no' } },
            { key: 'service_stamp_enabled', label: '服務印花', labelKey: 'stampSettingsPage.columns.serviceStampEnabled', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: 'stampSettingsPage.options.yes', false: 'stampSettingsPage.options.no' } },
            { key: 'amount_stamp_enabled', label: '金額印花', labelKey: 'stampSettingsPage.columns.amountStampEnabled', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: 'stampSettingsPage.options.yes', false: 'stampSettingsPage.options.no' } },
            { key: 'status', label: '狀態', labelKey: 'stampSettingsPage.columns.status', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        formFields: [
            { key: 'name', label: '活動名稱', labelKey: 'stampSettingsPage.fields.name', type: 'text', required: true, fullWidth: true },
            { key: 'description', label: '活動描述', labelKey: 'stampSettingsPage.fields.description', type: 'textarea', required: false, fullWidth: true },
            { key: 'status', label: '狀態', labelKey: 'stampSettingsPage.fields.status', type: 'select', required: false, fullWidth: true, defaultValue: 'active', options: [
                { value: 'active', label: '活躍', labelKey: 'stampSettingsPage.options.active' },
                { value: 'inactive', label: '停用', labelKey: 'stampSettingsPage.options.inactive' }
            ]},
            { key: 'valid_from', label: '開始日期', labelKey: 'stampSettingsPage.fields.validFrom', type: 'datetime-local', required: false },
            { key: 'valid_to', label: '結束日期', labelKey: 'stampSettingsPage.fields.validTo', type: 'datetime-local', required: false },
            // 產品印花設定
            { key: 'product_stamp_enabled', label: '購買特定產品獲得印花', labelKey: 'stampSettingsPage.fields.productStampEnabled', type: 'select', required: false, fullWidth: true, defaultValue: 'false', options: [
                { value: 'true', label: '是', labelKey: 'stampSettingsPage.options.yes' },
                { value: 'false', label: '否', labelKey: 'stampSettingsPage.options.no' }
            ]},
            { key: 'earning_products', label: '選擇可獲得印花的產品', labelKey: 'stampSettingsPage.fields.earningProducts', type: 'select2-multi', relationApi: '/products', relationLabel: 'name', required: false, fullWidth: true, 
              dependency: { field: 'product_stamp_enabled', value: 'true' },
              helpText: '選擇購買哪些產品可以獲得印花', helpTextKey: 'stampSettingsPage.fields.earningProductsHelp' },
            { key: 'product_stamp_count', label: '每產品獲得印花數', labelKey: 'stampSettingsPage.fields.productStampCount', type: 'number', required: false, defaultValue: 1, min: 1,
              dependency: { field: 'product_stamp_enabled', value: 'true' } },
            { key: 'product_stamp_daily_limit', label: '每日每產品上限（留空=無上限）', labelKey: 'stampSettingsPage.fields.productStampDailyLimit', type: 'number', required: false,
              dependency: { field: 'product_stamp_enabled', value: 'true' } },
            // 服務印花設定
            { key: 'service_stamp_enabled', label: '購買特定服務獲得印花', labelKey: 'stampSettingsPage.fields.serviceStampEnabled', type: 'select', required: false, fullWidth: true, defaultValue: 'false', options: [
                { value: 'true', label: '是', labelKey: 'stampSettingsPage.options.yes' },
                { value: 'false', label: '否', labelKey: 'stampSettingsPage.options.no' }
            ]},
            { key: 'earning_services', label: '選擇可獲得印花的服務', labelKey: 'stampSettingsPage.fields.earningServices', type: 'select2-multi', relationApi: '/services', relationLabel: 'name', required: false, fullWidth: true,
              dependency: { field: 'service_stamp_enabled', value: 'true' },
              helpText: '選擇購買哪些服務可以獲得印花', helpTextKey: 'stampSettingsPage.fields.earningServicesHelp' },
            { key: 'service_stamp_count', label: '每服務獲得印花數', labelKey: 'stampSettingsPage.fields.serviceStampCount', type: 'number', required: false, defaultValue: 1, min: 1,
              dependency: { field: 'service_stamp_enabled', value: 'true' } },
            { key: 'service_stamp_daily_limit', label: '每日每服務上限（留空=無上限）', labelKey: 'stampSettingsPage.fields.serviceStampDailyLimit', type: 'number', required: false,
              dependency: { field: 'service_stamp_enabled', value: 'true' } },
            // 金額印花設定
            { key: 'amount_stamp_enabled', label: '購買特定金額獲得印花', labelKey: 'stampSettingsPage.fields.amountStampEnabled', type: 'select', required: false, fullWidth: true, defaultValue: 'false', options: [
                { value: 'true', label: '是', labelKey: 'stampSettingsPage.options.yes' },
                { value: 'false', label: '否', labelKey: 'stampSettingsPage.options.no' }
            ]},
            { key: 'amount_per_stamp', label: '每消費多少金額獲得1印花', labelKey: 'stampSettingsPage.fields.amountPerStamp', type: 'number', required: false, step: '0.01', defaultValue: 100,
              dependency: { field: 'amount_stamp_enabled', value: 'true' } },
            { key: 'amount_stamp_daily_limit', label: '每日金額印花上限（留空=無上限）', labelKey: 'stampSettingsPage.fields.amountStampDailyLimit', type: 'number', required: false,
              dependency: { field: 'amount_stamp_enabled', value: 'true' } },
            // 可換購產品設定
            { key: 'redeemable_products', label: '可換購產品', labelKey: 'stampSettingsPage.fields.redeemableProducts', type: 'select2-multi', relationApi: '/products', relationLabel: 'name', required: false, fullWidth: true,
              helpText: '選擇可用印花兌換的產品', helpTextKey: 'stampSettingsPage.fields.redeemableProductsHelp' },
            { key: 'default_stamps_required', label: '預設所需印花數', labelKey: 'stampSettingsPage.fields.defaultStampsRequired', type: 'number', required: false, min: 1, defaultValue: 10,
              helpText: '新增可換購產品時的預設印花需求', helpTextKey: 'stampSettingsPage.fields.defaultStampsRequiredHelp' },
            { key: 'default_daily_limit', label: '預設每日換購上限（留空=無上限）', labelKey: 'stampSettingsPage.fields.defaultDailyLimit', type: 'number', required: false,
              helpText: '新增可換購產品時的預設每日上限', helpTextKey: 'stampSettingsPage.fields.defaultDailyLimitHelp' }
        ]
    },

    // 印花記錄
    'stamp-records': {
        title: '印花記錄',
        titleKey: 'stampRecordsPage.title',
        icon: 'bi-clock-history',
        apiPath: '/stamp-records',
        listPath: '/stamp-records',
        editPath: '/stamp-records',
        exportEnabled: true,
        hideAddButton: true,
        columns: [
            { key: 'customer.name', label: '客戶', labelKey: 'stampRecordsPage.columns.customer', type: 'relation' },
            { key: 'record_type', label: '類型', labelKey: 'stampRecordsPage.columns.recordType', type: 'badge', options: { earn: 'success', redeem: 'warning' }, labels: { earn: 'stampRecordsPage.options.earn', redeem: 'stampRecordsPage.options.redeem' } },
            { key: 'stamp_setting.name', label: '印花活動', labelKey: 'stampRecordsPage.columns.stampSetting', type: 'relation' },
            { key: 'stamp_count', label: '印花數', labelKey: 'stampRecordsPage.columns.stampCount', type: 'number' },
            { key: 'balance_after', label: '餘額', labelKey: 'stampRecordsPage.columns.balanceAfter', type: 'number' },
            { key: 'source_type', label: '來源', labelKey: 'stampRecordsPage.columns.sourceType', type: 'text' },
            { key: 'created_at', label: '時間', labelKey: 'stampRecordsPage.columns.createdAt', type: 'datetime' }
        ],
        filters: [
            { key: 'customer_id', label: '客戶', labelKey: 'stampRecordsPage.filters.customer', type: 'select2', relationApi: '/customers', relationLabel: 'name' },
            { key: 'stamp_setting_id', label: '印花活動', labelKey: 'stampRecordsPage.filters.stampSetting', type: 'select2', relationApi: '/stamp-settings', relationLabel: 'name' },
            { key: 'record_type', label: '類型', labelKey: 'stampRecordsPage.filters.recordType', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'stampRecordsPage.options.all' },
                { value: 'earn', label: '獲得', labelKey: 'stampRecordsPage.options.earn' },
                { value: 'redeem', label: '兌換', labelKey: 'stampRecordsPage.options.redeem' }
            ]}
        ],
        formFields: []
    },

    // 頁面管理
    'pages': {
        title: '頁面管理',
        icon: 'bi-file-earmark-text',
        apiPath: '/pages',
        listPath: '/pages',
        editPath: '/pages',
        exportEnabled: false,
        enableImportActions: false,
        columns: [
            { key: 'name', label: '頁面名稱', type: 'text' },
            { key: 'slug', label: '網址路徑', type: 'text' },
            { key: 'status', label: '狀態', type: 'badge', options: { draft: 'secondary', published: 'success' } },
            { key: 'is_homepage', label: '首頁', type: 'badge', options: { true: 'primary', false: 'secondary' } }
        ],
        searchPlaceholder: '搜索頁面名稱或路徑...',
        formFields: [
            { key: 'name', label: '頁面名稱', type: 'text', required: true, fullWidth: true },
            { key: 'slug', label: '網址路徑', type: 'text', required: true, fullWidth: true, placeholder: '例如：about-us' },
            { key: 'title', label: '頁面標題（SEO）', type: 'text', required: false, fullWidth: true },
            { key: 'description', label: '頁面描述', type: 'textarea', required: false, fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, fullWidth: true, defaultValue: 'draft', options: [
                { value: 'draft', label: '草稿' },
                { value: 'published', label: '已發布' }
            ]},
            { key: 'is_homepage', label: '設為首頁', type: 'select', required: false, fullWidth: true, defaultValue: 'false', options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'seo_title', label: 'SEO 標題', type: 'text', required: false, fullWidth: true },
            { key: 'seo_description', label: 'SEO 描述', type: 'textarea', required: false, fullWidth: true },
            { key: 'seo_keywords', label: 'SEO 關鍵字', type: 'text', required: false, fullWidth: true, placeholder: '用逗號分隔' }
        ]
    },

    // 產品類型
    'product-types': {
        title: '產品類型管理',
        icon: 'bi-tags',
        apiPath: '/product-types',
        listPath: '/product-types',
        editPath: '/product-types',
        exportEnabled: false,
        columns: [
            { key: 'image_url', label: '圖片', type: 'image', default: '/static/product.jpg' },
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'parent.name', label: '上層類型', type: 'relation' },
            { key: 'description', label: '描述', type: 'text' }
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'parent_id', label: '上層類型', type: 'select2', relationApi: '/product-types?root_only=true', relationLabel: 'name', required: false, placeholder: '選擇上層類型' },
            { key: 'image_url', label: '圖片', type: 'file', required: false, accept: 'image/*' },
            { key: 'description', label: '描述', type: 'textarea', required: false }
        ]
    },

    // 產品屬性
    'product-attributes': {
        title: '產品屬性管理',
        icon: 'bi-list-check',
        apiPath: '/product-attributes',
        listPath: '/product-attributes',
        editPath: '/product-attributes',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'attribute_type', label: '類型', type: 'badge', options: { 'input': 'primary', 'dropdown': 'info', 'select': 'info' }, labels: { 'input': '輸入框', 'dropdown': '下拉選單', 'select': '下拉選單' } },
            { key: 'options', label: '選項', type: 'text', format: (value) => {
                if (!value) return '-';
                if (Array.isArray(value)) {
                    return value.join(', ');
                }
                if (typeof value === 'string') {
                    return value;
                }
                return JSON.stringify(value);
            }},
            { key: 'is_required', label: '必填', type: 'badge', options: { true: 'danger', false: 'secondary' }, labels: { true: '是', false: '否' } },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'attribute_type', label: '類型', type: 'select', required: true, options: [
                { value: 'input', label: '輸入框 (Input)' },
                { value: 'dropdown', label: '下拉選單 (Dropdown)' }
            ], defaultValue: 'input' },
            { key: 'options', label: '選項', type: 'textarea', required: false, placeholder: '用逗號分隔選項，例如：紅色,藍色,綠色', helpText: '下拉選單（Dropdown）：用逗號分隔選項，例如：紅色,藍色,綠色\n輸入框（Input）：如果產品已有屬性值，輸入框會自動設為只讀（readonly）', dependency: { field: 'attribute_type', value: 'dropdown' } },
            { key: '_help_text', label: '', type: 'custom', render: () => {
                const t = (key, fallback) => {
                    try {
                        if (typeof I18n !== 'undefined' && I18n.t) {
                            const v = I18n.t(key);
                            if (v && v !== key) return v;
                        }
                    } catch (e) {
                        // ignore
                    }
                    return fallback;
                };
                return `
                    <div class="mb-3">
                        <div class="alert alert-info" style="font-size: 0.875rem;">
                            <strong>${t('productAttributes.help.title', '使用說明：')}</strong><br>
                            <strong>${t('productAttributes.help.dropdownTitle', '下拉選單（Dropdown）：')}</strong>${t('productAttributes.help.dropdownDesc', '在此字段輸入選項，用逗號分隔，例如：紅色,藍色,綠色')}<br>
                            <strong>${t('productAttributes.help.inputTitle', '輸入框（Input）：')}</strong>${t('productAttributes.help.inputDesc', '如果產品已有屬性值，輸入框會自動設為只讀（readonly），顯示灰色背景')}
                        </div>
                    </div>
                `;
            }},
            { key: 'is_required', label: '必填', type: 'select', required: false, options: [
                { value: 'yes', label: '是' },
                { value: 'no', label: '否' }
            ], defaultValue: 'no' },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: 'active' }
        ]
    },

    // 品牌
    'brands': {
        title: '品牌管理',
        icon: 'bi-award',
        apiPath: '/brands',
        listPath: '/brands',
        editPath: '/brands',
        exportEnabled: false,
        columns: [
            { key: 'logo_url', label: '圖片', type: 'image', default: '/static/product.jpg' },
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'description', label: '描述', type: 'text' }
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'logo_url', label: '圖片', type: 'file', required: false, accept: 'image/*' },
            { key: 'description', label: '描述', type: 'textarea', required: false }
        ]
    },

    // 服務種類
    'service-types': {
        title: '服務種類管理',
        icon: 'bi-list-ul',
        apiPath: '/service-types',
        listPath: '/service-types',
        editPath: '/service-types',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'description', label: '描述', type: 'text' }
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'description', label: '描述', type: 'textarea', required: false }
        ]
    },

    // 服務
    'services': {
        title: '服務管理',
        icon: 'bi-tools',
        apiPath: '/services',
        listPath: '/services',
        editPath: '/services',
        exportEnabled: true,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'price', label: '價格', type: 'currency' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'image_url', label: '圖片', type: 'file', required: false },
            { key: 'description', label: '描述', type: 'textarea', required: false },
            { key: 'price', label: '價格', type: 'number', required: false, step: '0.01' },
            { key: 'service_tax_ids', label: '服務稅 (多選)', type: 'select2-multi', relationApi: '/service-taxes', relationLabel: 'name', required: false, fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ]},
            { key: 'show_on_vmarket', label: '在 VMarket 顯示', type: 'checkbox', required: false, defaultValue: false, fullWidth: true, helpText: '加入 VMarket 後才會對外顯示', helpTextKey: 'services.helpTexts.show_on_vmarket' }
        ]
    },

    // 服務稅
    'service-taxes': {
        title: '服務稅',
        icon: 'bi-percent',
        apiPath: '/service-taxes',
        listPath: '/service-taxes',
        editPath: '/service-taxes',
        exportEnabled: true,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'code', label: '代碼', type: 'text' },
            { key: 'tax_mode', label: '模式', type: 'text' },
            { key: 'tax_value', label: '稅值', type: 'number' },
            { key: 'default_include', label: '服務單預設包含', type: 'default-include', options: [
                { value: 'service_order', label: '' }
            ] },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索服務稅名稱或代碼...',
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true, fullWidth: true },
            { key: 'code', label: '代碼', type: 'text', required: false, fullWidth: true },
            { key: 'tax_mode', label: '計算模式', type: 'select', required: true, defaultValue: 'percent', options: [
                { value: 'percent', label: '百分比 (%)' },
                { value: 'fixed', label: '固定金額' }
            ], fullWidth: true },
            { key: 'tax_value', label: '稅值', type: 'number', required: true, step: '0.0001', defaultValue: 0, fullWidth: true },
            { key: 'default_include', label: '服務單預設包含', type: 'default-include-single', includeValue: 'service_order', required: false, fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, defaultValue: 'active', options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '停用', labelKey: 'common.inactive' }
            ], fullWidth: true }
        ]
    },

    // 預約
    'appointments': {
        title: '預約管理',
        icon: 'bi-calendar-check',
        apiPath: '/appointments',
        listPath: '/appointments',
        editPath: '/appointments',
        exportEnabled: true,
        columns: [
            { key: 'customer.name', label: '客戶', type: 'relation' },
            { key: 'start_time', label: '開始時間', type: 'datetime' },
            { key: 'end_time', label: '結束時間', type: 'datetime' },
            { key: 'service.name', label: '服務', type: 'relation' },
            { key: 'service.service_type.name', label: '服務種類', type: 'relation' },
            { key: 'staff.name', label: '服務員', type: 'relation' },
            { key: 'status', label: '狀態', type: 'badge', options: { confirmed: 'success', cancelled: 'secondary', completed: 'info' }, labels: { confirmed: '已確認', cancelled: '已取消', completed: '已完成' } }
        ],
        filters: [
            { key: 'service_type_id', label: '服務種類', labelKey: 'fields.service_type_id', type: 'select2', relationApi: '/service-types', relationLabel: 'name' },
            { key: 'staff_id', label: '服務員', labelKey: 'fields.staff_id', type: 'select2', relationApi: '/service-staffs', relationLabel: 'name' },
            { key: 'customer_id', label: '客戶', labelKey: 'fields.customer_id', type: 'select2', relationApi: '/customers', relationLabel: 'name' },
            { key: 'status', label: '狀態', labelKey: 'fields.status', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'confirmed', label: '已確認', labelKey: 'options.status.confirmed' },
                { value: 'completed', label: '已完成', labelKey: 'options.status.completed' },
                { value: 'cancelled', label: '已取消', labelKey: 'options.status.cancelled' }
            ]}
        ],
        formFields: [
            // 客戶可選（允許私人預約）
            { key: 'customer_id', label: '客戶', type: 'select2', relationApi: '/customers', relationLabel: 'name', required: false },
            { key: 'service_id', label: '服務', type: 'select2', relationApi: '/services', relationLabel: 'name', required: false },
            { key: 'service_order_id', label: '關聯服務單', type: 'select2', relationApi: '/service-orders', relationLabel: 'order_number', required: false },
            { key: 'staff_id', label: '服務員', type: 'select2', relationApi: '/service-staffs', relationLabel: 'name', required: false },
            { key: 'start_time', label: '開始時間', type: 'datetime-local', required: true },
            { key: 'end_time', label: '結束時間', type: 'datetime-local', required: true },
            { key: 'reminder_time', label: '提示時間', type: 'datetime-local', required: false, sameRow: 'status' },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'confirmed', label: '已預約' },
                { value: 'cancelled', label: '已取消' },
                { value: 'completed', label: '已完成' }
            ], sameRow: 'reminder_time' },
            // 房間、車輛、設備、備註放在表單最底部
            { key: 'room_ids', label: '房間', type: 'select2-multi', relationApi: '/rooms', relationLabel: 'name', required: false, fullWidth: true },
            { key: 'vehicle_ids', label: '車輛', type: 'select2-multi', relationApi: '/vehicles', relationLabel: 'name', required: false, fullWidth: true },
            { key: 'equipment_ids', label: '設備', type: 'select2-multi', relationApi: '/equipments', relationLabel: 'name', required: false, fullWidth: true },
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    // 服務單
    'service-orders': {
        title: '服務單管理',
        icon: 'bi-clipboard-check',
        apiPath: '/service-orders',
        listPath: '/service-orders',
        editPath: '/service-orders',
        exportEnabled: true,
        labelFilter: { apiPath: '/service-order-labels', paramKey: 'label_id', defaultShow: 4 },
        moreActions: [
            { id: 'importFromExcel', label: '匯入 Excel', icon: 'bi bi-upload' },
            { id: 'downloadImportTemplate', label: '下載匯入 Excel（不含資料）', icon: 'bi bi-download', arg: false },
            { id: 'downloadImportTemplate', label: '下載匯入 Excel（含資料）', icon: 'bi bi-download', arg: true },
            { type: 'divider' }
        ],
        columns: [
            { key: 'order_number', label: '服務單號', type: 'text' },
            { key: 'service_types', label: '服務種類', type: 'tags' },
            { key: 'services', label: '服務', type: 'tags' },
            // 用 customer 物件渲染（支援 last_name 顯示）
            { key: 'customer', label: '客戶', type: 'relation', relationKey: 'name' },
            { key: 'labels', label: '標籤', type: 'labels' },
            { key: 'service_date', label: '服務日期', type: 'date' },
            { key: 'total_amount', label: '總金額', type: 'currency' },
            { key: 'status', label: '狀態', type: 'badge', options: { draft: 'secondary', confirmed: 'info', processing: 'warning', completed: 'success', cancelled: 'danger' }, labels: { draft: '草稿', confirmed: '已確認', processing: '處理中', completed: '已完成', cancelled: '已取消' } },
            { key: 'salesperson.name', label: '銷售員', type: 'relation' },
            { key: 'created_at', label: '創建時間', type: 'datetime' }
        ],
        searchPlaceholder: '搜索服務單號、客戶名稱、服務名稱...',
        filters: [
            { key: 'service_type_id', label: '服務種類', type: 'select2', relationApi: '/service-types', relationLabel: 'name' },
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'confirmed', label: '已確認' },
                { value: 'processing', label: '處理中' },
                { value: 'completed', label: '已完成' },
                { value: 'cancelled', label: '已取消' }
            ]}
        ],
        formFields: [
            { key: 'order_number', label: '服務單號', type: 'text', required: false, readonly: true },
            { key: 'customer_id', label: '客戶', type: 'select2', relationApi: '/customers', relationLabel: 'name', required: false },
            { key: 'service_date', label: '服務日期', type: 'date', required: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'confirmed', label: '已確認' },
                { value: 'processing', label: '處理中' },
                { value: 'completed', label: '已完成' },
                { value: 'cancelled', label: '已取消' }
            ], defaultValue: 'confirmed' },
            { key: 'salesperson_id', label: '銷售員', type: 'select2', relationApi: '/users', relationLabel: 'name', required: false },
            { key: 'store_id', label: '所屬店舖', type: 'select2', relationApi: '/stores', relationLabel: 'name', required: false },
            { key: 'contact_name', label: '聯絡人姓名', type: 'text', required: false },
            { key: 'contact_email', label: '聯絡人電郵', type: 'email', required: false },
            { key: 'contact_phone', label: '聯絡電話', type: 'text', required: false },
            { key: 'contact_address', label: '聯絡地址', type: 'textarea', required: false },
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    // 出租單管理（列表頁使用 dynamic_list，新增/編輯頁使用自定義模板 rental_orders_new.html）
    'rental-orders': {
        title: '出租單管理',
        icon: 'bi-building',
        apiPath: '/rental-orders',
        listPath: '/rental-orders',
        editPath: '/rental-orders',
        exportEnabled: false,
        labelFilter: { apiPath: '/rental-order-labels', paramKey: 'label_id', defaultShow: 4 },
        columns: [
            { key: 'order_number', label: '出租單號', type: 'text' },
            { key: 'resources', label: '資源', type: 'tags' },
            { key: 'customer', label: '客戶', type: 'relation', relationKey: 'name' },
            { key: 'labels', label: '標籤', type: 'labels' },
            { key: 'rental_date', label: '出租日期', type: 'date' },
            { key: 'total_amount', label: '總金額', type: 'currency' },
            { key: 'status', label: '狀態', type: 'badge', options: { draft: 'secondary', confirmed: 'info', processing: 'warning', completed: 'success', cancelled: 'danger' }, labels: { draft: '草稿', confirmed: '已確認', processing: '處理中', completed: '已完成', cancelled: '已取消' } },
            { key: 'salesperson.name', label: '銷售員', type: 'relation' },
            { key: 'created_at', label: '創建時間', type: 'datetime' }
        ],
        searchPlaceholder: '搜索出租單號、客戶名稱、資源名稱...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'confirmed', label: '已確認' },
                { value: 'processing', label: '處理中' },
                { value: 'completed', label: '已完成' },
                { value: 'cancelled', label: '已取消' }
            ]}
        ],
        formFields: [
            { key: 'order_number', label: '出租單號', type: 'text', required: false, readonly: true },
            { key: 'customer_id', label: '客戶', type: 'select2', relationApi: '/customers', relationLabel: 'name', required: false },
            { key: 'rental_date', label: '出租日期', type: 'date', required: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'confirmed', label: '已確認' },
                { value: 'processing', label: '處理中' },
                { value: 'completed', label: '已完成' },
                { value: 'cancelled', label: '已取消' }
            ], defaultValue: 'confirmed' },
            { key: 'salesperson_id', label: '銷售員', type: 'select2', relationApi: '/users', relationLabel: 'name', required: false },
            { key: 'store_id', label: '所屬店舖', type: 'select2', relationApi: '/stores', relationLabel: 'name', required: false },
            { key: 'contact_name', label: '聯絡人姓名', type: 'text', required: false },
            { key: 'contact_email', label: '聯絡人電郵', type: 'email', required: false },
            { key: 'contact_phone', label: '聯絡電話', type: 'text', required: false },
            { key: 'contact_address', label: '聯絡地址', type: 'textarea', required: false },
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    // 店舖管理
    stores: {
        title: '店舖管理',
        icon: 'bi-shop',
        apiPath: '/stores',
        listPath: '/stores',
        editPath: '/stores',
        exportEnabled: true,
        columns: [
            { key: 'image_url', label: '圖片', type: 'image', default: '/static/product.jpg' },
            { key: 'code', label: '編號', type: 'text' },
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'address', label: '地址', type: 'text' },
            { key: 'contact_person', label: '聯絡人', type: 'text' },
            { key: 'phone', label: '電話', type: 'text' },
            { key: 'email', label: '郵箱', type: 'email' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索店舖名稱、編號...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ]}
        ],
        formFields: [
            { key: 'code', label: '編號', type: 'text', required: false, readonly: true },
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'image_url', label: '圖片', type: 'file', required: false, accept: 'image/*' },
            { key: 'contact_person', label: '聯絡人', type: 'text', required: false },
            { key: 'phone_country_code', label: '電話區號', type: 'select2', relationApi: '/api/v1/phone-country-codes', relationLabel: 'code', relationValueKey: 'code', required: false, defaultValue: '+852' },
            { key: 'phone', label: '電話', type: 'text', required: false },
            { key: 'email', label: '郵箱', type: 'email', required: false },
            { key: 'address_country_code', label: '國家', type: 'select2', relationApi: '/api/v1/countries', relationLabel: 'name', relationValueKey: 'code', required: false },
            { key: 'address_region_code', label: '地區', type: 'select2', relationApi: '/api/v1/country-regions', relationLabel: 'name', relationValueKey: 'code', required: false },
            { key: 'address', label: '地址', type: 'textarea', required: false },
            { key: 'status', label: '狀態', type: 'select', required: false, defaultValue: 'active', options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ]}
        ]
    },

    // 餐桌區管理
    'dining-areas': {
        title: '餐桌區管理',
        icon: 'bi-grid-3x3-gap',
        apiPath: '/dining-areas',
        listPath: '/dining-areas',
        editPath: '/dining-areas',
        exportEnabled: false,
        columns: [
            { key: 'code', label: '區號', type: 'text' },
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'min_seats', label: '最少人數', type: 'number' },
            { key: 'max_seats', label: '最多人數', type: 'number' },
            { key: 'store.name', label: '所屬店舖', type: 'relation' },
            { key: 'is_active', label: '狀態', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: 'common.active', false: 'common.inactive' } }
        ],
        searchPlaceholder: '搜索桌區名稱或代碼...',
        filters: [
            { key: 'is_active', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: true, label: '活躍', labelKey: 'common.active' },
                { value: false, label: '非活躍', labelKey: 'common.inactive' }
            ]}
        ],
        formFields: [
            { key: 'code', label: '區號', type: 'text', required: true },
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'store_id', label: '所屬店舖', type: 'select2', relationApi: '/stores', relationLabel: 'name', required: false },
            { key: 'min_seats', label: '最少人數', type: 'number', required: false, defaultValue: 1 },
            { key: 'max_seats', label: '最多人數', type: 'number', required: false, defaultValue: 1 },
            { key: 'sort_order', label: '排序', type: 'number', required: false, defaultValue: 0 },
            { key: 'notes', label: '備註', type: 'textarea', required: false },
            { key: 'is_active', label: '狀態', type: 'select', required: false, options: [
                { value: true, label: '活躍', labelKey: 'common.active' },
                { value: false, label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: true, fullWidth: true }
        ]
    },

    // 餐桌管理
    'dining-tables': {
        title: '餐桌管理',
        icon: 'bi-table',
        apiPath: '/dining-tables',
        listPath: '/dining-tables',
        editPath: '/dining-tables',
        exportEnabled: false,
        columns: [
            { key: 'code', label: '桌號', type: 'text' },
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'area', label: '桌區', type: 'relation', relationKey: 'name' },
            { key: 'seats', label: '人數', type: 'number' },
            { key: 'status', label: '狀態', type: 'badge', options: { available: 'success', occupied: 'danger', cleaning: 'warning', reserved: 'info' }, labels: { available: '空桌', occupied: '使用中', cleaning: '清潔中', reserved: '已預約' } },
            { key: 'store.name', label: '所屬店舖', type: 'relation' },
            { key: 'is_active', label: '啟用狀態', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: 'common.active', false: 'common.inactive' } }
        ],
        searchPlaceholder: '搜索桌號或名稱...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'available', label: '空桌' },
                { value: 'occupied', label: '使用中' },
                { value: 'cleaning', label: '清潔中' },
                { value: 'reserved', label: '已預約' }
            ]},
            { key: 'is_active', label: '啟用狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: true, label: '活躍', labelKey: 'common.active' },
                { value: false, label: '非活躍', labelKey: 'common.inactive' }
            ]}
        ],
        formFields: [
            { key: 'code', label: '桌號', type: 'text', required: true },
            { key: 'name', label: '名稱', type: 'text', required: false },
            { key: 'area_id', label: '桌區', type: 'select2', relationApi: '/dining-areas', relationLabel: 'name', required: true },
            { key: 'store_id', label: '所屬店舖', type: 'select2', relationApi: '/stores', relationLabel: 'name', required: false },
            { key: 'seats', label: '人數', type: 'number', required: false, defaultValue: 1 },
            { key: 'status', label: '餐桌狀態', type: 'select', required: false, defaultValue: 'available', options: [
                { value: 'available', label: '空桌' },
                { value: 'occupied', label: '使用中' },
                { value: 'cleaning', label: '清潔中' },
                { value: 'reserved', label: '已預約' }
            ]},
            { key: 'notes', label: '備註', type: 'textarea', required: false },
            { key: 'is_active', label: '啟用狀態', type: 'select', required: false, options: [
                { value: true, label: '活躍', labelKey: 'common.active' },
                { value: false, label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: true, fullWidth: true }
        ]
    },

    // 候位排隊
    'dining-queues': {
        title: '候位排隊',
        icon: 'bi-people',
        apiPath: '/dining-queues',
        listPath: '/dining-queues',
        editPath: '/dining-queues',
        exportEnabled: false,
        columns: [
            { key: 'ticket_number', label: '票號', type: 'text' },
            { key: 'name', label: '姓名', type: 'text' },
            { key: 'phone', label: '電話', type: 'text' },
            { key: 'party_size', label: '人數', type: 'number' },
            { key: 'area.name', label: '桌區', type: 'relation' },
            { key: 'table', label: '餐桌', type: 'relation', relationKey: 'code' },
            { key: 'status', label: '狀態', type: 'badge', options: { waiting: 'warning', seated: 'success', cancelled: 'secondary' }, labels: { waiting: '等待中', seated: '已入座', cancelled: '已取消' } },
            { key: 'store.name', label: '所屬店舖', type: 'relation' }
        ],
        searchPlaceholder: '搜索姓名或電話...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'waiting', label: '等待中' },
                { value: 'seated', label: '已入座' },
                { value: 'cancelled', label: '已取消' }
            ]}
        ],
        formFields: [
            { key: 'ticket_number', label: '候位編號', type: 'text', required: false, readonly: true, fullWidth: true, placeholder: '請先選擇桌區' },
            { key: 'name', label: '姓名（可選）', type: 'text', required: false },
            { key: 'phone', label: '電話', type: 'text', required: false },
            { key: 'party_size', label: '人數', type: 'number', required: false, defaultValue: 1 },
            { key: 'status', label: '狀態', type: 'select', required: false, defaultValue: 'waiting', options: [
                { value: 'waiting', label: '等待中' },
                { value: 'seated', label: '已入座' },
                { value: 'cancelled', label: '已取消' }
            ]},
            { key: 'table_id', label: '餐桌', type: 'select2', relationApi: '/dining-tables', relationLabel: 'code', required: false },
            { key: 'area_id', label: '桌區', type: 'select2', relationApi: '/dining-areas', relationLabel: 'name', required: false },
            { key: 'store_id', label: '所屬店舖', type: 'select2', relationApi: '/stores', relationLabel: 'name', required: false },
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    // 採購單
    'purchase-orders': {
        title: '採購單管理',
        icon: 'bi-cart-plus',
        apiPath: '/purchase-orders',
        listPath: '/purchase-orders',
        editPath: '/purchase-orders',
        exportEnabled: true,
        labelFilter: { apiPath: '/purchase-order-labels', paramKey: 'label_id', defaultShow: 4 },
        moreActions: [
            { id: 'importFromExcel', label: '匯入 Excel', icon: 'bi bi-upload' },
            { id: 'downloadImportTemplate', label: '下載匯入 Excel（不含資料）', icon: 'bi bi-download', arg: false },
            { id: 'downloadImportTemplate', label: '下載匯入 Excel（含資料）', icon: 'bi bi-download', arg: true },
            { type: 'divider' }
        ],
        columns: [
            { key: 'order_number', label: '採購單號', type: 'text' },
            { key: 'supplier', label: '供應商', type: 'relation', relationKey: 'name' },
            { key: 'labels', label: '標籤', type: 'labels' },
            { key: 'order_date', label: '日期', type: 'date' },
            { key: 'final_amount', label: '金額', type: 'currency' },
            { key: 'status', label: '狀態', type: 'badge', options: { 
                confirmed: 'info',
                cancelled: 'danger',
                completed: 'success'
            }, labels: {
                confirmed: '已確認',
                cancelled: '已取消',
                completed: '已完成'
            }}
        ],
        searchPlaceholder: '搜索採購單號或供應商名稱...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部狀態', labelKey: 'common.allStatuses' },
                { value: 'confirmed', label: '已確認' },
                { value: 'cancelled', label: '已取消' },
                { value: 'completed', label: '已完成' }
            ]}
        ],
        formFields: [
            { key: 'supplier_id', label: '供應商', type: 'select', required: true, 
              relationApi: '/suppliers', relationLabel: 'name' },
            { key: 'order_date', label: '訂單日期', type: 'date', required: true },
            { key: 'expected_delivery_date', label: '預計交貨日期', type: 'date', required: false },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'confirmed', label: '已確認' },
                { value: 'cancelled', label: '已取消' },
                { value: 'completed', label: '已完成' }
            ], defaultValue: 'confirmed'},
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    // 客服通訊
    'support-communications': {
        title: '聯絡表單',
        icon: 'bi-envelope-paper',
        apiPath: '/support-communications',
        listPath: '/support-communications',
        editPath: '/support-communications',
        exportEnabled: false,
        columns: [
            { key: 'created_at', label: '時間', type: 'datetime' },
            { key: 'customer.name', label: '客戶', type: 'relation', fallback: '未註冊客戶' },
            { key: 'subject', label: '主題', type: 'text' },
            { key: 'communication_type', label: '類型', type: 'badge', options: { contact_form: 'info', email: 'primary', phone: 'success', message: 'warning', other: 'secondary' }, labels: { contact_form: '聯絡表單', email: '郵件', phone: '電話', message: '訊息', other: '其他' } },
            { key: 'status', label: '狀態', type: 'badge', options: { open: 'warning', in_progress: 'info', resolved: 'success', closed: 'secondary' }, labels: { open: '待處理', in_progress: '處理中', resolved: '已解決', closed: '已關閉' } },
            { key: 'priority', label: '優先級', type: 'badge', options: { low: 'secondary', normal: 'info', high: 'warning', urgent: 'danger' }, labels: { low: '低', normal: '一般', high: '高', urgent: '緊急' } },
            { key: 'content', label: '內容', type: 'text', maxLength: 100, showTooltip: true }
        ],
        formFields: [
            { key: 'customer_id', label: '客戶', type: 'select2', relationApi: '/customers', relationLabel: 'name', relationValueKey: 'id', required: false, fullWidth: true },
            { key: 'subject', label: '主題', type: 'text', required: true, fullWidth: true },
            { key: 'communication_type', label: '通訊類型', type: 'select', required: true, defaultValue: 'contact_form', fullWidth: true, options: [
                { value: 'contact_form', label: '聯絡表單' },
                { value: 'email', label: '郵件' },
                { value: 'phone', label: '電話' },
                { value: 'message', label: '訊息' },
                { value: 'other', label: '其他' }
            ]},
            { key: 'content', label: '內容', type: 'textarea', required: true, rows: 6 },
            { key: 'direction', label: '方向', type: 'select', required: true, defaultValue: 'inbound', options: [
                { value: 'inbound', label: '來電/來信' },
                { value: 'outbound', label: '去電/去信' }
            ]},
            { key: 'status', label: '狀態', type: 'select', required: true, defaultValue: 'open', options: [
                { value: 'open', label: '待處理' },
                { value: 'in_progress', label: '處理中' },
                { value: 'resolved', label: '已解決' },
                { value: 'closed', label: '已關閉' }
            ]},
            { key: 'priority', label: '優先級', type: 'select', required: true, defaultValue: 'normal', options: [
                { value: 'low', label: '低' },
                { value: 'normal', label: '一般' },
                { value: 'high', label: '高' },
                { value: 'urgent', label: '緊急' }
            ]},
            { key: 'staff_id', label: '負責員工', type: 'select2', relationApi: '/users', relationLabel: 'name', relationValueKey: 'id', required: false }
        ]
    },

    // 推廣發送
    'promotions': {
        title: '推廣發送管理',
        icon: 'bi-megaphone',
        apiPath: '/promotions',
        listPath: '/promotions',
        editPath: '/promotions',
        exportEnabled: false,
        columns: [
            { key: 'title', label: '標題', type: 'text' },
            { key: 'promotion_type', label: '發送模式', type: 'badge', options: { message: 'info', whatsapp: 'success' }, labels: { message: 'Message', whatsapp: 'Whatsapp' } },
            { key: 'status', label: '狀態', type: 'badge', options: { draft: 'secondary', scheduled: 'warning', sending: 'info', sent: 'success', cancelled: 'danger' }, labels: { draft: '草稿', scheduled: '已排程', sending: '發送中', sent: '已發送', cancelled: '已取消' } },
            { key: 'total_recipients', label: '收件人數', type: 'number' },
            { key: 'sent_at', label: '發送時間', type: 'datetime' }
        ],
        formFields: [
            { key: 'title', label: '標題', type: 'text', required: true },
            { key: 'promotion_type', label: '發送模式', type: 'select', required: true, options: [
                { value: 'message', label: 'Message (發送給客戶)' },
                { value: 'whatsapp', label: 'Whatsapp (稍後連接 API)' }
            ]},
            { key: 'send_type', label: '發送方式', type: 'select', required: true, options: [
                { value: 'immediate', label: '即時發送' },
                { value: 'scheduled', label: '排程發送' }
            ], onChange: 'togglePromotionSchedule', fullWidth: true },
            { key: 'content', label: '發送內容', type: 'textarea', required: true, placeholder: '輸入要發送的內容，可添加 emoji 😊', rows: 6 },
            { key: 'target_audience', label: '目標受眾', type: 'textarea', required: false, placeholder: 'JSON格式：{"member_levels": ["gold"], "status": "active"}' },
            { key: 'scheduled_at', label: '排程時間', type: 'datetime-local', required: false, dependency: { field: 'send_type', value: 'scheduled' } },
            { key: 'status', label: '狀態', type: 'select', required: false, readonly: true, default: 'scheduled', options: [
                { value: 'scheduled', label: '已排程' },
                { value: 'sending', label: '發送中' },
                { value: 'sent', label: '已發送' },
                { value: 'cancelled', label: '已取消' }
            ], fullWidth: true }
        ]
    },

    // 服務員
    'service-staffs': {
        title: '服務員管理',
        icon: 'bi-person-badge',
        apiPath: '/service-staffs',
        listPath: '/service-staffs',
        editPath: '/service-staffs',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '姓名', type: 'text' },
            { key: 'phone', label: '電話', type: 'text' },
            { key: 'service_type.name', label: '服務類別', type: 'relation' },
            { key: 'employee_number', label: '員工編號', type: 'text' },
            { key: 'hourly_rate', label: '時薪', type: 'currency' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        filters: [
            { key: 'service_type_id', label: '服務類別', type: 'select2', relationApi: '/service-types', relationLabel: 'name' },
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部' },
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ]}
        ],
        formFields: [
            { key: 'name', label: '姓名', type: 'text', required: true },
            { key: 'phone', label: '電話', type: 'text', required: false },
            { key: 'service_type_id', label: '所屬服務類別', type: 'select2', relationApi: '/service-types', relationLabel: 'name', required: false },
            { key: 'employee_number', label: '員工編號', type: 'select2', relationApi: '/users', relationLabel: 'employee_number', relationValueKey: 'employee_number', relationDisplayFormat: 'employee_number-name', required: false },
            { key: 'specialization', label: '專長', type: 'textarea', required: false },
            { key: 'hourly_rate', label: '時薪', type: 'number', required: false, step: '0.01' },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ]}
        ]
    },

    // 房間設備
    'rooms': {
        title: '房間管理',
        icon: 'bi-door-open',
        apiPath: '/rooms',
        listPath: '/rooms',
        editPath: '/rooms',
        exportEnabled: true,
        columns: [
            { key: 'image_url', label: '圖片', type: 'image', default: '/static/product.jpg' },
            { key: 'code', label: '編號', type: 'text' },
            { key: 'name', label: '房間名稱', type: 'text' },
            { key: 'capacity', label: '容量', type: 'number' },
            { key: 'status', label: '狀態', type: 'badge', options: { available: 'success', in_use: 'warning', maintenance: 'danger', unavailable: 'secondary' } },
            { key: 'allow_overlap', label: '允許重複使用', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } }
        ],
        searchPlaceholder: '搜索房間名稱或編號...',
        formFields: [
            { key: 'code', label: '編號', type: 'text', required: false, readonly: true },
            { key: 'name', label: '房間名稱', type: 'text', required: true },
            { key: 'image_url', label: '圖片', type: 'file', required: false, accept: 'image/*', fullWidth: true },
            { key: 'description', label: '描述', type: 'textarea', required: false },
            { key: 'capacity', label: '容量', type: 'number', required: false },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'available', label: '可用' },
                { value: 'in_use', label: '使用中' },
                { value: 'maintenance', label: '維護中' },
                { value: 'unavailable', label: '不可用' }
            ]},
            { key: 'allow_overlap', label: '允許重複使用', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    'equipments': {
        title: '設備管理',
        icon: 'bi-gear',
        apiPath: '/equipments',
        listPath: '/equipments',
        editPath: '/equipments',
        exportEnabled: true,
        columns: [
            { key: 'image_url', label: '圖片', type: 'image', default: '/static/product.jpg' },
            { key: 'code', label: '編號', type: 'text' },
            { key: 'name', label: '設備名稱', type: 'text' },
            { key: 'status', label: '狀態', type: 'badge', options: { available: 'success', in_use: 'warning', maintenance: 'danger', unavailable: 'secondary' } },
            { key: 'allow_overlap', label: '允許重複使用', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } }
        ],
        searchPlaceholder: '搜索設備名稱或編號...',
        formFields: [
            { key: 'code', label: '編號', type: 'text', required: false, readonly: true },
            { key: 'name', label: '設備名稱', type: 'text', required: true },
            { key: 'image_url', label: '圖片', type: 'file', required: false, accept: 'image/*', fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'available', label: '可用' },
                { value: 'in_use', label: '使用中' },
                { value: 'maintenance', label: '維護中' },
                { value: 'unavailable', label: '不可用' }
            ]},
            { key: 'allow_overlap', label: '允許重複使用', type: 'select', required: false, defaultValue: 'false', options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    'vehicles': {
        title: '車輛管理',
        icon: 'bi-car-front',
        apiPath: '/vehicles',
        listPath: '/vehicles',
        editPath: '/vehicles',
        exportEnabled: true,
        columns: [
            { key: 'image_url', label: '圖片', type: 'image', default: '/static/product.jpg' },
            { key: 'code', label: '編號', type: 'text' },
            { key: 'name', label: '車輛名稱', type: 'text' },
            { key: 'vehicle_type', label: '車輛類型', type: 'text' },
            { key: 'license_plate', label: '車牌號碼', type: 'text' },
            { key: 'status', label: '狀態', type: 'badge', options: { available: 'success', in_use: 'warning', maintenance: 'danger', unavailable: 'secondary' } },
            { key: 'allow_overlap', label: '允許重複使用', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } }
        ],
        searchPlaceholder: '搜索車輛名稱、編號或車牌號碼...',
        formFields: [
            { key: 'code', label: '編號', type: 'text', required: false, readonly: true },
            { key: 'name', label: '車輛名稱', type: 'text', required: true },
            { key: 'image_url', label: '圖片', type: 'file', required: false, accept: 'image/*' },
            { key: 'vehicle_type', label: '車輛類型', type: 'text', required: false },
            { key: 'license_plate', label: '車牌號碼', type: 'text', required: false },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'available', label: '可用' },
                { value: 'in_use', label: '使用中' },
                { value: 'maintenance', label: '維護中' },
                { value: 'unavailable', label: '不可用' }
            ]},
            { key: 'allow_overlap', label: '允許重複使用', type: 'select', required: false, defaultValue: 'false', fullWidth: true, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    'room-equipments': {
        title: '房間設備管理',
        icon: 'bi-door-open',
        apiPath: '/room-equipments',
        listPath: '/room-equipments',
        editPath: '/room-equipments',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'location', label: '位置', type: 'text' }
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'location', label: '位置', type: 'text', required: false }
        ]
    },

    // 訂單報表
    'order-reports': {
        title: '訂單報表',
        icon: 'bi-file-earmark-text',
        apiPath: '/order-reports',
        listPath: '/order-reports',
        editPath: '/order-reports',
        exportEnabled: true,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'period_start', label: '開始日期', type: 'date' },
            { key: 'period_end', label: '結束日期', type: 'date' }
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'period_start', label: '開始日期', type: 'date', required: false },
            { key: 'period_end', label: '結束日期', type: 'date', required: false },
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    // 收入
    'incomes': {
        title: '收入管理',
        icon: 'bi-arrow-down-circle',
        apiPath: '/incomes',
        listPath: '/incomes',
        editPath: '/incomes',
        exportEnabled: true,
        columns: [
            { key: 'description', label: '標題', type: 'text' },
            { key: 'category', label: '類別', type: 'badge', options: { other: 'secondary', order: 'info', service_order: 'primary' } },
            { key: 'amount', label: '金額', type: 'currency' },
            { key: 'income_date', label: '日期', type: 'date' }
        ],
        formFields: [
            { key: 'description', label: '標題', type: 'text', required: true, fullWidth: true },
            { key: 'related_user_id', label: '相關人員', type: 'select2', relationApi: '/users', relationLabel: 'name', required: false, fullWidth: true },
            { key: 'category', label: '類別', type: 'select', required: true, options: [
                { value: 'other', label: '其他' },
                { value: 'order', label: '訂單' },
                { value: 'service_order', label: '服務單' }
            ], defaultValue: 'other', fullWidth: true },
            { key: 'reference_id', label: '關聯訂單', type: 'select2', relationApi: '/orders', relationLabel: 'order_number', relationValueKey: 'id', relationLabelKey: 'order_number', required: false, dependency: { field: 'category', values: ['order'] }, fullWidth: true },
            { key: 'reference_id', label: '關聯服務單', type: 'select2', relationApi: '/service-orders', relationLabel: 'order_number', relationValueKey: 'id', relationLabelKey: 'order_number', required: false, dependency: { field: 'category', values: ['service_order'] }, fullWidth: true },
            { key: 'amount', label: '金額', type: 'number', required: true, step: '0.01' },
            { key: 'income_date', label: '日期', type: 'date', required: true },
            { key: 'payment_method', label: '付款方法', type: 'select2', relationApi: '/payment-methods', relationLabel: 'name', relationValueKey: 'id', relationLabelKey: 'name', required: false },
            { key: 'reference_number', label: '參考號碼', type: 'text', required: false },
            { key: 'bank_account_id', label: '收款賬戶(可選輸入)', type: 'select2', relationApi: '/bank-accounts', relationLabel: 'name', relationValueKey: 'id', relationLabelKey: 'name', relationLabelFields: ['name','account_number'], required: false, fullWidth: true },
            { key: 'attachment', label: '附件', type: 'file', required: false, accept: '*/*' },
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    // 會計科目表
    'accounts': {
        title: '會計科目表',
        icon: 'bi-diagram-3',
        apiPath: '/accounts',
        listPath: '/accounts',
        editPath: '/accounts',
        exportEnabled: true,
        columns: [
            { key: 'code', label: '科目代碼', type: 'text' },
            { key: 'name', label: '科目名稱', type: 'text' },
            { key: 'account_type', label: '類型', type: 'badge', options: { asset: 'primary', liability: 'danger', equity: 'info', revenue: 'success', expense: 'warning' } },
            { key: 'sub_type', label: '子類型', type: 'text' },
            { key: 'is_system', label: '系統科目', type: 'badge', options: { true: 'secondary', false: 'light' }, labels: { true: '是', false: '否' } },
            { key: 'is_active', label: '啟用', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } }
        ],
        searchPlaceholder: '搜索科目代碼或名稱...',
        filters: [
            { key: 'account_type', label: '類型', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'asset', label: '資產' },
                { value: 'liability', label: '負債' },
                { value: 'equity', label: '權益' },
                { value: 'revenue', label: '收入' },
                { value: 'expense', label: '費用' }
            ] },
            { key: 'is_active', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'true', label: '啟用' },
                { value: 'false', label: '停用' }
            ] }
        ],
        formFields: [
            { key: 'parent_id', label: '上級科目', type: 'select2', relationApi: '/accounts?all=true', relationLabel: 'name', relationValueKey: 'id', relationLabelFields: ['code', 'name'], required: false, fullWidth: true },
            { key: 'code', label: '科目代碼', type: 'text', required: true },
            { key: 'name', label: '科目名稱', type: 'text', required: true },
            { key: 'account_type', label: '類型', type: 'select', required: true, options: [
                { value: 'asset', label: '資產' },
                { value: 'liability', label: '負債' },
                { value: 'equity', label: '權益' },
                { value: 'revenue', label: '收入' },
                { value: 'expense', label: '費用' }
            ], defaultValue: 'expense' },
            { key: 'sub_type', label: '子類型', type: 'text', required: false },
            { key: 'currency', label: '幣別', type: 'text', required: false, placeholder: '例如 HKD / USD（空白=跟隨租戶）' },
            { key: 'tax_rate', label: '預設稅率(%)', type: 'number', required: false, step: '0.0001', defaultValue: 0 },
            { key: 'sort_order', label: '排序', type: 'number', required: false, defaultValue: 0 },
            { key: 'description', label: '描述', type: 'textarea', required: false, fullWidth: true },
            { key: 'is_active', label: '啟用', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ], defaultValue: 'true' }
        ]
    },

    // 日記帳
    'journal-entries': {
        title: '日記帳',
        icon: 'bi-journal-text',
        apiPath: '/journal-entries',
        listPath: '/journal-entries',
        editPath: '/journal-entries',
        exportEnabled: true,
        columns: [
            { key: 'entry_number', label: '分錄編號', type: 'text' },
            { key: 'entry_date', label: '日期', type: 'date' },
            { key: 'description', label: '描述', type: 'text' },
            { key: 'status', label: '狀態', type: 'badge', options: { draft: 'secondary', posted: 'success', void: 'danger' } },
            { key: 'total_debit', label: '借方總額', type: 'currency' },
            { key: 'total_credit', label: '貸方總額', type: 'currency' }
        ],
        searchPlaceholder: '搜索分錄編號或描述...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'draft', label: '草稿' },
                { value: 'posted', label: '已過帳' },
                { value: 'void', label: '已作廢' }
            ] }
        ],
        formFields: [
            { key: 'entry_number', label: '分錄編號（留空自動生成）', type: 'text', required: false, fullWidth: true },
            { key: 'entry_date', label: '日期', type: 'date', required: true },
            { key: 'description', label: '描述', type: 'text', required: true, fullWidth: true },
            { key: 'reference_type', label: '來源類型', type: 'select', required: false, options: [
                { value: '', label: '手動' },
                { value: 'income', label: '收入' },
                { value: 'expense', label: '支出' },
                { value: 'invoice', label: '發票' },
                { value: 'purchase', label: '採購' },
                { value: 'manual', label: '手動' }
            ], defaultValue: 'manual' },
            { key: 'debit_account_id', label: '借方科目', type: 'select2', relationApi: '/accounts?all=true', relationLabel: 'name', relationValueKey: 'id', relationLabelFields: ['code', 'name'], required: true, fullWidth: true },
            { key: 'credit_account_id', label: '貸方科目', type: 'select2', relationApi: '/accounts?all=true', relationLabel: 'name', relationValueKey: 'id', relationLabelFields: ['code', 'name'], required: true, fullWidth: true },
            { key: 'amount', label: '金額', type: 'number', required: true, step: '0.01' },
            { key: 'debit_description', label: '借方說明', type: 'text', required: false, fullWidth: true },
            { key: 'credit_description', label: '貸方說明', type: 'text', required: false, fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'draft', label: '草稿' },
                { value: 'posted', label: '已過帳' },
                { value: 'void', label: '已作廢' }
            ], defaultValue: 'posted' }
        ]
    },

    // 稅務配置
    'tax-configs': {
        title: '稅務配置',
        icon: 'bi-receipt-cutoff',
        apiPath: '/tax-configs',
        listPath: '/tax-configs',
        editPath: '/tax-configs',
        exportEnabled: true,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'code', label: '代碼', type: 'text' },
            { key: 'region', label: '地區', type: 'text' },
            { key: 'tax_type', label: '稅種', type: 'badge', options: { sales_tax: 'primary', purchase_tax: 'info', income_tax: 'warning', vat: 'success', gst: 'success' } },
            { key: 'rate', label: '稅率(%)', type: 'number' },
            { key: 'is_default', label: '預設', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } },
            { key: 'is_active', label: '啟用', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } }
        ],
        searchPlaceholder: '搜索稅務設定...',
        filters: [
            { key: 'tax_type', label: '稅種', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'sales_tax', label: '銷項稅' },
                { value: 'purchase_tax', label: '進項稅' },
                { value: 'income_tax', label: '所得稅' },
                { value: 'vat', label: 'VAT/GST' }
            ] }
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'code', label: '代碼', type: 'text', required: false },
            { key: 'region', label: '地區', type: 'text', required: false, placeholder: '例如: HK, TW, SG, GLOBAL' },
            { key: 'tax_type', label: '稅種', type: 'select', required: true, options: [
                { value: 'sales_tax', label: '銷項稅' },
                { value: 'purchase_tax', label: '進項稅' },
                { value: 'income_tax', label: '所得稅' },
                { value: 'vat', label: 'VAT' },
                { value: 'gst', label: 'GST' }
            ], defaultValue: 'sales_tax' },
            { key: 'rate', label: '稅率(%)', type: 'number', required: true, step: '0.0001', defaultValue: 0 },
            { key: 'account_id', label: '對應科目', type: 'select2', relationApi: '/accounts?all=true', relationLabel: 'name', relationValueKey: 'id', relationLabelFields: ['code', 'name'], required: false, fullWidth: true },
            { key: 'is_default', label: '設為預設', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ], defaultValue: 'false' },
            { key: 'is_active', label: '啟用', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ], defaultValue: 'true' },
            { key: 'description', label: '說明', type: 'textarea', required: false, fullWidth: true }
        ]
    },

    // 自動過帳規則
    'posting-rules': {
        title: '自動過帳規則',
        icon: 'bi-diagram-2',
        apiPath: '/posting-rules',
        listPath: '/posting-rules',
        editPath: '/posting-rules',
        exportEnabled: true,
        columns: [
            { key: 'source_type', label: '來源', type: 'badge', options: { income: 'success', expense: 'danger' } },
            { key: 'category', label: '類別', type: 'text' },
            { key: 'debit_account.name', label: '借方科目', type: 'relation' },
            { key: 'credit_account.name', label: '貸方科目', type: 'relation' },
            { key: 'is_system', label: '系統規則', type: 'badge', options: { true: 'secondary', false: 'light' }, labels: { true: '是', false: '否' } },
            { key: 'is_active', label: '啟用', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } }
        ],
        searchPlaceholder: '搜索來源或類別...',
        filters: [
            { key: 'source_type', label: '來源', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'income', label: '收入' },
                { value: 'expense', label: '支出' }
            ] }
        ],
        formFields: [
            { key: 'source_type', label: '來源', type: 'select', required: true, options: [
                { value: 'income', label: '收入' },
                { value: 'expense', label: '支出' }
            ], defaultValue: 'income' },
            { key: 'category', label: '類別（可用 * 表示通用）', type: 'text', required: true, defaultValue: '*' },
            { key: 'debit_account_id', label: '借方科目', type: 'select2', relationApi: '/accounts?all=true', relationLabel: 'name', relationValueKey: 'id', relationLabelFields: ['code', 'name'], required: true, fullWidth: true },
            { key: 'credit_account_id', label: '貸方科目', type: 'select2', relationApi: '/accounts?all=true', relationLabel: 'name', relationValueKey: 'id', relationLabelFields: ['code', 'name'], required: true, fullWidth: true },
            { key: 'sort_order', label: '優先順序（越小越優先）', type: 'number', required: false, defaultValue: 0 },
            { key: 'is_active', label: '啟用', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ], defaultValue: 'true' },
            { key: 'description', label: '說明', type: 'textarea', required: false, fullWidth: true }
        ]
    },

    // 支出
    'expenses': {
        title: '支出管理',
        icon: 'bi-arrow-up-circle',
        apiPath: '/expenses',
        listPath: '/expenses',
        editPath: '/expenses',
        exportEnabled: true,
        columns: [
            { key: 'description', label: '描述', type: 'text' },
            { key: 'amount', label: '金額', type: 'currency' },
            { key: 'expense_date', label: '日期', type: 'date' },
            { key: 'expense_type', label: '類型', type: 'badge', options: { purchase: 'info', refund: 'warning', salary: 'warning', rent: 'danger', utility: 'primary', order_commission: 'success', service_order_commission: 'success', product_tax: 'dark', service_tax: 'dark', other: 'secondary' } }
        ],
        filters: [
            { key: 'project_id', label: '項目', type: 'select2', relationApi: '/projects', relationLabel: 'name', required: false, options: [
                { value: '', label: '全部', labelKey: 'common.all' }
            ] }
        ],
        formFields: [
            { key: 'description', label: '標題', type: 'text', required: true, fullWidth: true },
            { key: 'related_user_id', label: '相關人員', type: 'select2', relationApi: '/users', relationLabel: 'name', required: false, fullWidth: true },
            { key: 'category', label: '類別', type: 'select', required: true, options: [
                { value: 'purchase', label: '採購' },
                { value: 'refund', label: '退款' },
                { value: 'project', label: '項目支出' },
                { value: 'order_commission', label: '訂單傭金' },
                { value: 'service_order_commission', label: '服務單傭金' },
                { value: 'product_tax', label: '產品稅' },
                { value: 'service_tax', label: '服務稅' },
                { value: 'other', label: '其他' }
            ], defaultValue: 'other', fullWidth: true },
            { key: 'project_id', label: '關聯項目', type: 'select2', relationApi: '/projects', relationLabel: 'name', required: false, fullWidth: true, dependency: { field: 'category', values: ['project'] } },
            { key: 'reference_id', label: '關聯採購單', type: 'select2', relationApi: '/purchase-orders', relationLabel: 'order_number', relationValueKey: 'id', relationLabelKey: 'order_number', required: false, dependency: { field: 'category', values: ['purchase'] }, fullWidth: true },
            { key: 'reference_id', label: '關聯訂單', type: 'select2', relationApi: '/orders', relationLabel: 'order_number', relationValueKey: 'id', relationLabelKey: 'order_number', required: false, dependency: { field: 'category', values: ['order_commission', 'product_tax', 'refund'] }, fullWidth: true },
            { key: 'reference_id', label: '關聯服務單', type: 'select2', relationApi: '/service-orders', relationLabel: 'order_number', relationValueKey: 'id', relationLabelKey: 'order_number', required: false, dependency: { field: 'category', values: ['service_order_commission', 'service_tax'] }, fullWidth: true },
            { key: 'amount', label: '金額', type: 'number', required: true, step: '0.01' },
            { key: 'expense_date', label: '日期', type: 'date', required: true },
            { key: 'payment_method', label: '支付方式', type: 'select2', relationApi: '/payment-methods', relationLabel: 'name', relationValueKey: 'id', relationLabelKey: 'name', required: false },
            { key: 'reference_number', label: '參考號碼', type: 'text', required: false },
            { key: 'bank_account_id', label: '付款賬戶(可選輸入)', type: 'select2', relationApi: '/bank-accounts', relationLabel: 'name', relationValueKey: 'id', relationLabelKey: 'name', relationLabelFields: ['name','account_number'], required: false, fullWidth: true },
            { key: 'attachment', label: '附件', type: 'file', required: false, accept: '*/*' },
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    // 員工（原使用者管理）
    'users': {
        title: '員工',
        icon: 'bi-people',
        apiPath: '/users',
        listPath: '/users',
        editPath: '/users',
        exportEnabled: true,
        columns: [
            { key: 'name', label: '姓名', type: 'text-with-avatar' },
            { key: 'email', label: '郵箱', type: 'email' },
            { key: 'role.name', label: '角色', type: 'relation' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } },
            { key: 'department.name', label: '部門', type: 'relation' },
            { key: 'salary', label: '薪資', type: 'currency' },
            { key: 'last_login_at', label: '最後登錄', type: 'date' }
        ],
        formFields: [
            { key: 'employee_number', label: '員工編號', type: 'text', required: false, readonly: true, fullWidth: true },
            { key: 'profile_pic', label: '頭像', type: 'profile-image', required: false, fullWidth: true },
            { key: 'name', label: '姓名', type: 'text', required: true, maxlength: 50, fullWidth: true },
            { key: 'email', label: '郵箱', type: 'email', required: true, fullWidth: true },
            { key: 'birth_date', label: '出生日期', type: 'date', required: false, fullWidth: true },
            { key: 'phone_country_code', label: '電話區號', type: 'select2', relationApi: '/api/v1/phone-country-codes', relationLabel: 'code', relationValueKey: 'code', required: false, defaultValue: '+852' },
            { key: 'phone', label: '電話', type: 'text', required: false },
            { key: 'password', label: '密碼', type: 'password', required: false, minlength: 6, maxlength: 20, fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, defaultValue: 'active', options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ]},
            { key: 'department_id', label: '部門', type: 'select2', relationApi: '/departments', relationLabel: 'name', required: false },
            { key: 'role_id', label: '角色', type: 'select2', relationApi: '/roles', relationLabel: 'name', required: true, fullWidth: true },
            { key: 'salary_mode', label: '薪資方式', type: 'select', required: false, options: [
                { value: 'monthly', label: '月薪' },
                { value: 'hourly', label: '時薪' }
            ], defaultValue: 'monthly', fullWidth: true },
            { key: 'salary', label: '薪資', type: 'number', required: false, step: '0.01', fullWidth: true },
            { key: 'order_commission_mode', label: '訂單佣金方式', type: 'select', required: false, options: [
                { value: 'percent', label: '%（按訂單總額）' },
                { value: 'fixed', label: '實額（每單固定）' }
            ], defaultValue: 'percent', fullWidth: true },
            { key: 'order_commission_rate', label: '訂單佣金率 (%)', type: 'number', required: false, step: '0.01', placeholder: '例如: 5.00 表示 5%', dependency: { field: 'order_commission_mode', values: ['percent'] }, fullWidth: true },
            { key: 'order_commission_fixed', label: '訂單佣金（實額）', type: 'number', required: false, step: '0.01', placeholder: '例如: 50 表示每單固定 $50', dependency: { field: 'order_commission_mode', values: ['fixed'] }, fullWidth: true },
            { key: 'service_order_commission_mode', label: '服務單佣金方式', type: 'select', required: false, options: [
                { value: 'percent', label: '%（按服務單總額）' },
                { value: 'fixed', label: '實額（每單固定）' }
            ], defaultValue: 'percent', fullWidth: true },
            { key: 'service_order_commission_rate', label: '服務單佣金率 (%)', type: 'number', required: false, step: '0.01', placeholder: '例如: 5.00 表示 5%', dependency: { field: 'service_order_commission_mode', values: ['percent'] }, fullWidth: true },
            { key: 'service_order_commission_fixed', label: '服務單佣金（實額）', type: 'number', required: false, step: '0.01', placeholder: '例如: 50 表示每單固定 $50', dependency: { field: 'service_order_commission_mode', values: ['fixed'] }, fullWidth: true },
            { key: 'shift_id', label: '工作時段', type: 'select2', relationApi: '/shifts', relationLabel: 'name', required: false, fullWidth: true }
        ]
    },

    // 庫存調整
    'inventory-adjustments': {
        title: '庫存調整',
        icon: 'bi-arrow-left-right',
        apiPath: '/inventory-adjustments',
        listPath: '/inventory-adjustments',
        editPath: '/inventory-adjustments',
        exportEnabled: true,
        columns: [
            { key: 'product.name', label: '產品', type: 'relation' },
            { key: 'warehouse.name', label: '倉庫', type: 'relation' },
            { key: 'adjustment_type', label: '調整類型', type: 'badge', options: { increase: 'success', decrease: 'danger', set: 'info' }, labels: { increase: '增加', decrease: '減少', set: '設定' } },
            { key: 'previous_quantity', label: '調整前', type: 'number' },
            { key: 'quantity', label: '調整數量', type: 'number' },
            { key: 'new_quantity', label: '調整後', type: 'number' },
            { key: 'reason', label: '原因', type: 'text' },
            { key: 'created_at', label: '調整時間', type: 'date' }
        ],
        searchPlaceholder: '搜索產品名稱或原因...',
        filters: [
            { key: 'adjustment_type', label: '調整類型', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'increase', label: '增加' },
                { value: 'decrease', label: '減少' },
                { value: 'set', label: '設定' }
            ]}
        ],
        formFields: [
            { key: 'product_id', label: '產品', type: 'select2', relationApi: '/products', relationLabel: 'name', required: true },
            { key: 'warehouse_id', label: '倉庫', type: 'select2', relationApi: '/warehouses', relationKey: 'id', relationValueKey: 'name', required: true },
            { key: 'adjustment_type', label: '調整類型', type: 'select', required: true, options: [
                { value: 'increase', label: '增加' },
                { value: 'decrease', label: '減少' },
                { value: 'set', label: '設定' }
            ]},
            { key: 'quantity', label: '數量', type: 'number', required: true, step: '1' },
            { key: 'reason', label: '原因', type: 'text', required: false },
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    // 庫存盤點（產品和庫存的匯總報告，不使用動態列表）
    'inventory-counts': {
        title: '庫存盤點',
        icon: 'bi-clipboard-check',
        apiPath: '/inventory-counts',
        listPath: '/inventory-counts',
        editPath: '/inventory-counts',
        exportEnabled: false,
        showAddButton: false, // 不顯示新增按鈕
        columns: [],
        formFields: []
    },

    // 低庫存預警
    'low-stock': {
        title: '低庫存預警',
        icon: 'bi-exclamation-triangle',
        apiPath: '/inventory/low-stock',
        listPath: '/inventory/low-stock',
        editPath: '/inventory/low-stock',
        exportEnabled: true,
        showActions: false, // 不顯示操作列
        showDraftButton: false, // 不顯示草稿按鈕
        columns: [
            { key: 'code', label: '編號', type: 'text' },
            { key: 'name', label: '產品名稱', type: 'text' },
            { key: 'stock_quantity', label: '當前庫存', type: 'number' },
            { key: 'price', label: '價格', type: 'currency' },
            { key: 'category', label: '分類', type: 'text' }
        ],
        searchPlaceholder: '搜索產品名稱或編號...',
        formFields: []
    },

    // 打卡記錄
    'attendances': {
        title: '打卡記錄',
        icon: 'bi-clock-history',
        apiPath: '/attendances',
        listPath: '/attendances',
        editPath: '/attendances',
        exportEnabled: true,
        columns: [
            { key: 'user.name', label: '員工', type: 'relation' },
            { key: 'date', label: '日期', type: 'date' },
            { key: 'clock_in', label: '上班時間', type: 'datetime' },
            { key: 'clock_out', label: '下班時間', type: 'datetime' },
            { key: 'work_duration', label: '工作時長（分鐘）', type: 'number' },
            { key: 'ot_duration', label: '加班時長（分鐘）', type: 'number' },
            { key: 'status', label: '狀態', type: 'badge', options: { normal: 'success', late: 'warning', early_leave: 'danger', absent: 'secondary' }, labels: { normal: '正常', late: '遲到', early_leave: '早退', absent: '缺勤' } }
        ],
        searchPlaceholder: '搜索員工名稱...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'normal', label: '正常' },
                { value: 'late', label: '遲到' },
                { value: 'early_leave', label: '早退' },
                { value: 'absent', label: '缺勤' }
            ]}
        ],
        formFields: [
            { key: 'user_id', label: '員工', type: 'select2', relationApi: '/users', relationLabel: 'name', required: true },
            { key: 'date', label: '日期', type: 'date', required: true },
            { key: 'clock_in', label: '上班時間', type: 'datetime-local', required: false },
            { key: 'clock_out', label: '下班時間', type: 'datetime-local', required: false },
            { key: 'status', label: '狀態', type: 'select', required: false, fullWidth: true, options: [
                { value: 'normal', label: '正常' },
                { value: 'late', label: '遲到' },
                { value: 'early_leave', label: '早退' },
                { value: 'absent', label: '缺勤' }
            ], defaultValue: 'normal' },
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    // 請假申請
    'leave-requests': {
        title: '請假申請',
        icon: 'bi-calendar-x',
        apiPath: '/leave-requests',
        listPath: '/leave-requests',
        editPath: '/leave-requests',
        exportEnabled: true,
        columns: [
            { key: 'user.name', label: '員工', type: 'relation' },
            { key: 'leave_type', label: '請假類型', type: 'badge', options: { annual: 'info', sick: 'warning', personal: 'primary', unpaid: 'secondary' }, labels: { annual: '年假', sick: '病假', personal: '事假', unpaid: '無薪假' } },
            { key: 'start_date', label: '開始日期', type: 'date' },
            { key: 'end_date', label: '結束日期', type: 'date' },
            { key: 'days', label: '天數', type: 'number' },
            { key: 'status', label: '狀態', type: 'badge', options: { pending: 'warning', approved: 'success', rejected: 'danger', cancelled: 'secondary' }, labels: { pending: '待審批', approved: '已批准', rejected: '已拒絕', cancelled: '已取消' } }
        ],
        searchPlaceholder: '搜索員工名稱...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'pending', label: '待審批' },
                { value: 'approved', label: '已批准' },
                { value: 'rejected', label: '已拒絕' },
                { value: 'cancelled', label: '已取消' }
            ]},
            { key: 'leave_type', label: '請假類型', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'annual', label: '年假' },
                { value: 'sick', label: '病假' },
                { value: 'personal', label: '事假' },
                { value: 'unpaid', label: '無薪假' }
            ]}
        ],
        formFields: [
            { key: 'leave_type', label: '請假類型', type: 'select', required: true, options: [
                { value: 'annual', label: '年假' },
                { value: 'sick', label: '病假' },
                { value: 'personal', label: '事假' },
                { value: 'unpaid', label: '無薪假' }
            ]},
            { key: 'start_date', label: '開始日期', type: 'date', required: true },
            { key: 'end_date', label: '結束日期', type: 'date', required: true },
            { key: 'reason', label: '原因', type: 'textarea', required: false }
        ]
    },

    // 假期
    'holidays': {
        title: '假期',
        icon: 'bi-calendar-check',
        apiPath: '/holidays',
        listPath: '/holidays',
        editPath: '/holidays',
        exportEnabled: true,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'start_date', label: '開始日期', type: 'date' },
            { key: 'end_date', label: '結束日期', type: 'date' },
            { key: 'is_recurring', label: '每年重複', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' }, labels: { active: '啟用', inactive: '停用' } }
        ],
        searchPlaceholder: '搜索假期名稱...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.allStatuses' },
                { value: 'active', label: '啟用', labelKey: 'common.active' },
                { value: 'inactive', label: '停用', labelKey: 'common.inactive' }
            ]}
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true, fullWidth: true },
            { key: 'description', label: '描述', type: 'textarea', required: false },
            { key: 'start_date', label: '開始日期', type: 'date', required: true },
            { key: 'end_date', label: '結束日期', type: 'date', required: true },
            { key: 'is_recurring', label: '每年重複', type: 'select', required: false, fullWidth: true, options: [
                { value: 'false', label: '否' },
                { value: 'true', label: '是' }
            ]},
            { key: 'status', label: '狀態', type: 'select', required: false, fullWidth: true, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: 'active' }
        ]
    },

    // 薪資記錄
    'payrolls': {
        title: '薪資記錄',
        icon: 'bi-cash-stack',
        apiPath: '/payrolls',
        listPath: '/payrolls',
        editPath: '/payrolls',
        exportEnabled: true,
        columns: [
            { key: 'user.name', label: '員工', type: 'relation' },
            { key: 'pay_period', label: '發薪期間', type: 'text' },
            { key: 'base_salary', label: '基本薪金', type: 'currency' },
            { key: 'ot_hours', label: '加班時數', type: 'number' },
            { key: 'ot_amount', label: '加班費', type: 'currency' },
            { key: 'mpf_total', label: '強積金總額', type: 'currency' },
            { key: 'gross_salary', label: '總薪金', type: 'currency' },
            { key: 'net_salary', label: '淨薪金', type: 'currency' },
            { key: 'status', label: '狀態', type: 'badge', options: { draft: 'secondary', confirmed: 'info', paid: 'success' }, labels: { draft: '草稿', confirmed: '已確認', paid: '已發放' } }
        ],
        searchPlaceholder: '搜索員工名稱...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'draft', label: '草稿' },
                { value: 'confirmed', label: '已確認' },
                { value: 'paid', label: '已發放' }
            ]}
        ],
        formFields: [
            { key: 'user_id', label: '員工', type: 'select2', relationApi: '/users', relationKey: 'id', relationValueKey: 'name', required: true },
            { key: 'pay_period', label: '發薪期間 (YYYY-MM)', type: 'text', required: true, placeholder: '例如: 2024-01' },
            { key: 'base_salary', label: '基本薪金', type: 'number', required: true, step: '0.01' },
            { key: 'ot_hours', label: '加班時數', type: 'number', required: false, step: '0.01' },
            { key: 'ot_rate', label: '加班倍率', type: 'number', required: false, step: '0.01', placeholder: '默認1.5倍' },
            { key: 'allowances', label: '津貼', type: 'number', required: false, step: '0.01' },
            { key: 'deductions', label: '扣除', type: 'number', required: false, step: '0.01' },
            { key: 'status', label: '狀態', type: 'select', required: false, fullWidth: true, options: [
                { value: 'draft', label: '草稿' },
                { value: 'confirmed', label: '已確認' },
                { value: 'paid', label: '已發放' }
            ], defaultValue: 'draft' },
            { key: 'notes', label: '備註', type: 'textarea', required: false }
        ]
    },

    // 薪資附加項目 presets（HR）
    'payroll_adjustment_presets': {
        title: '薪資附加項目',
        icon: 'bi-sliders',
        apiPath: '/payroll-adjustment-presets',
        listPath: '/payroll-adjustment-presets',
        editPath: '/payroll-adjustment-presets',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'direction', label: '加/減', type: 'badge', options: { add: 'success', subtract: 'danger' }, labels: { add: '加', subtract: '減' } },
            { key: 'mode', label: '模式', type: 'badge', options: { fixed: 'secondary', percent: 'primary' }, labels: { fixed: '實額', percent: '%' } },
            { key: 'rate_percent', label: '百分比(%)', type: 'number' },
            { key: 'amount', label: '實額', type: 'currency' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' }, labels: { active: '啟用', inactive: '停用' } }
        ],
        searchPlaceholder: '搜索名稱...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'active', label: '啟用', labelKey: 'common.active' },
                { value: 'inactive', label: '停用', labelKey: 'common.inactive' }
            ]}
        ],
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true, fullWidth: true },
            { key: 'direction', label: '加/減', type: 'select', required: true, fullWidth: true, options: [
                { value: 'add', label: '加' },
                { value: 'subtract', label: '減' }
            ], defaultValue: 'add' },
            { key: 'mode', label: '模式', type: 'select', required: true, fullWidth: true, options: [
                { value: 'fixed', label: '實額' },
                { value: 'percent', label: '%（基本薪金）' }
            ], defaultValue: 'fixed' },
            { key: 'rate_percent', label: '百分比(%)', type: 'number', required: false, step: '0.01', placeholder: '例如：5 代表 5%' },
            { key: 'amount', label: '實額', type: 'number', required: false, step: '0.01', placeholder: '例如：500' },
            { key: 'status', label: '狀態', type: 'select', required: false, fullWidth: true, options: [
                { value: 'active', label: '啟用', labelKey: 'common.active' },
                { value: 'inactive', label: '停用', labelKey: 'common.inactive' }
            ], defaultValue: 'active' }
        ]
    },

    // HR：空缺
    'job_vacancies': {
        title: '空缺',
        icon: 'bi-briefcase',
        apiPath: '/job-vacancies',
        listPath: '/job-vacancies',
        editPath: '/job-vacancies',
        exportEnabled: false,
        columns: [
            { key: 'title', label: '職位', type: 'text' },
            { key: 'department', label: '部門', type: 'relation', relationKey: 'name' },
            { key: 'headcount', label: '人數', type: 'number' },
            { key: 'status', label: '狀態', type: 'badge', options: { open: 'success', closed: 'secondary' }, labels: { open: '開放', closed: '關閉' } },
            { key: 'created_at', label: '建立時間', type: 'datetime' }
        ],
        searchPlaceholder: '搜索職位...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'open', label: '開放' },
                { value: 'closed', label: '關閉' }
            ]}
        ],
        formFields: [
            { key: 'title', label: '職位', type: 'text', required: true, fullWidth: true },
            { key: 'department_id', label: '部門', type: 'select2', relationApi: '/departments', relationLabel: 'name', required: false, fullWidth: true },
            { key: 'headcount', label: '人數', type: 'number', required: false, defaultValue: 1 },
            { key: 'status', label: '狀態', type: 'select', required: false, fullWidth: true, options: [
                { value: 'open', label: '開放' },
                { value: 'closed', label: '關閉' }
            ], defaultValue: 'open' },
            { key: 'description', label: '描述', type: 'textarea', required: false, fullWidth: true }
        ]
    },

    // HR：聘請
    'job_hires': {
        title: '聘請',
        icon: 'bi-person-plus',
        apiPath: '/job-hires',
        listPath: '/job-hires',
        editPath: '/job-hires',
        exportEnabled: false,
        columns: [
            { key: 'candidate_display_name', label: '求職者', type: 'text' },
            { key: 'vacancy', label: '空缺', type: 'relation', relationKey: 'title' },
            { key: 'status', label: '狀態', type: 'badge', options: { applied: 'secondary', interview: 'info', offered: 'warning', hired: 'success', rejected: 'danger' }, labels: { applied: '已申請', interview: '面試', offered: '已發 offer', hired: '已聘請', rejected: '拒絕' } },
            { key: 'start_date', label: '入職日期', type: 'date', disableKeyTranslation: true },
            { key: 'created_at', label: '建立時間', type: 'datetime' }
        ],
        searchPlaceholder: '搜索求職者/電郵/電話...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'applied', label: '已申請' },
                { value: 'interview', label: '面試' },
                { value: 'offered', label: '已發 offer' },
                { value: 'hired', label: '已聘請' },
                { value: 'rejected', label: '拒絕' }
            ]}
        ],
        formFields: [
            { key: 'vacancy_id', label: '空缺', type: 'select2', relationApi: '/job-vacancies', relationLabel: 'title', required: false, fullWidth: true },
            { key: 'applicant_id', label: '求職者', type: 'select2', relationApi: '/job-applicants', relationLabel: 'candidate_display_name', required: false, fullWidth: true },
            { key: 'candidate_name', label: '名字', type: 'text', required: true },
            { key: 'candidate_last_name', label: '姓氏（可選）', type: 'text', required: false, placeholder: '例如：周、Chow' },
            { key: 'email', label: '電郵', type: 'email', required: false },
            { key: 'phone', label: '電話', type: 'text', required: false },
            { key: 'status', label: '狀態', type: 'select', required: false, fullWidth: true, options: [
                { value: 'applied', label: '已申請' },
                { value: 'interview', label: '面試' },
                { value: 'offered', label: '已發 offer' },
                { value: 'hired', label: '已聘請' },
                { value: 'rejected', label: '拒絕' }
            ], defaultValue: 'applied' },
            { key: 'start_date', label: '入職日期', type: 'date', required: false, disableKeyTranslation: true },
            { key: 'notes', label: '備註', type: 'textarea', required: false, fullWidth: true }
        ]
    },

    // HR：求職者 / 候選人
    'job_applicants': {
        title: '求職者',
        icon: 'bi-person-lines-fill',
        apiPath: '/job-applicants',
        listPath: '/job-applicants',
        editPath: '/job-applicants',
        exportEnabled: false,
        columns: [
            { key: 'candidate_display_name', label: '候選人', type: 'text' },
            { key: 'vacancy', label: '空缺', type: 'relation', relationKey: 'title' },
            { key: 'email', label: '電郵', type: 'text' },
            { key: 'phone', label: '電話', type: 'text' },
            // 頭像放在「狀態」前一欄
            { key: 'profile_pic', label: '頭像', type: 'profile-image' },
            { key: 'status', label: '狀態', type: 'badge', options: { applied: 'secondary', interview: 'info', offered: 'warning', hired: 'success', rejected: 'danger' }, labels: { applied: '已申請', interview: '面試', offered: '已發 offer', hired: '已聘請', rejected: '拒絕' } },
            { key: 'created_at', label: '建立時間', type: 'datetime' }
        ],
        searchPlaceholder: '搜索候選人/電郵/電話...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'applied', label: '已申請' },
                { value: 'interview', label: '面試' },
                { value: 'offered', label: '已發 offer' },
                { value: 'hired', label: '已聘請' },
                { value: 'rejected', label: '拒絕' }
            ]}
        ],
        formFields: [
            { key: 'vacancy_id', label: '空缺', type: 'select2', relationApi: '/job-vacancies', relationLabel: 'title', required: false, fullWidth: true },
            { key: 'candidate_name', label: '名字', type: 'text', required: true },
            { key: 'candidate_last_name', label: '姓氏（可選）', type: 'text', required: false, placeholder: '例如：周、Chow' },
            { key: 'email', label: '電郵', type: 'email', required: false },
            { key: 'phone', label: '電話', type: 'text', required: false },
            { key: 'profile_pic', label: '頭像', type: 'profile-image', required: false, fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, fullWidth: true, options: [
                { value: 'applied', label: '已申請' },
                { value: 'interview', label: '面試' },
                { value: 'offered', label: '已發 offer' },
                { value: 'hired', label: '已聘請' },
                { value: 'rejected', label: '拒絕' }
            ], defaultValue: 'applied' },
            { key: 'notes', label: '備註', type: 'textarea', required: false, fullWidth: true }
        ]
    },

    // 倉庫管理
    warehouses: {
        title: '倉庫管理',
        icon: 'bi-building',
        apiPath: '/warehouses',
        listPath: '/warehouses',
        editPath: '/warehouses',
        exportEnabled: true,
        columns: [
            { key: 'code', label: '編號', type: 'text' },
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'address', label: '地址', type: 'text' },
            { key: 'contact_person', label: '聯絡人', type: 'text' },
            { key: 'phone', label: '電話', type: 'text' },
            { key: 'email', label: '郵箱', type: 'text' },
            { key: 'is_default', label: '系統預設', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' }, labels: { active: '啟用', inactive: '停用' } }
        ],
        searchPlaceholder: '搜索倉庫編號或名稱...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.allStatuses' },
                { value: 'active', label: '啟用', labelKey: 'common.active' },
                { value: 'inactive', label: '停用', labelKey: 'common.inactive' }
            ]}
        ],
        formFields: [
            { key: 'code', label: '編號', type: 'text', required: true, readonly: true },
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'address_country_code', label: '國家', type: 'select2', relationApi: '/api/v1/countries', relationLabel: 'name', relationValueKey: 'code', required: false },
            { key: 'address_region_code', label: '地區', type: 'select2', relationApi: '/api/v1/country-regions', relationLabel: 'name', relationValueKey: 'code', required: false },
            { key: 'address', label: '地址', type: 'textarea' },
            { key: 'contact_person', label: '聯絡人', type: 'text' },
            { key: 'phone_country_code', label: '電話區號', type: 'select2', relationApi: '/api/v1/phone-country-codes', relationLabel: 'code', relationValueKey: 'code', required: false, defaultValue: '+852' },
            { key: 'phone', label: '電話', type: 'text', required: false },
            { key: 'email', label: '郵箱', type: 'email' },
            { key: 'is_default', label: '系統預設', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'status', label: '狀態', type: 'select', required: true, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: 'active'}
        ]
    },

    // 訂單標籤管理
    'order-labels': {
        title: '訂單標籤管理',
        icon: 'bi-tags',
        apiPath: '/order-labels',
        listPath: '/order-labels',
        editPath: '/order-labels',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'color', label: '顏色', type: 'color' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索標籤名稱...',
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'color', label: '顏色', type: 'color', required: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: 'active'}
        ]
    },

    // 產品標籤管理
    'product-labels': {
        title: '產品標籤管理',
        icon: 'bi-tags',
        apiPath: '/product-labels',
        listPath: '/product-labels',
        editPath: '/product-labels',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'color', label: '顏色', type: 'color' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索標籤名稱...',
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'color', label: '顏色', type: 'color', required: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: 'active'}
        ]
    },

    // 服務單標籤管理
    'service-order-labels': {
        title: '服務單標籤管理',
        icon: 'bi-tags',
        apiPath: '/service-order-labels',
        listPath: '/service-order-labels',
        editPath: '/service-order-labels',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'color', label: '顏色', type: 'color' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索標籤名稱...',
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true, fullWidth: true },
            { key: 'color', label: '顏色', type: 'color', required: true, fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: 'active', fullWidth: true}
        ]
    },

    // 出租單標籤管理
    'rental-order-labels': {
        title: '出租單標籤管理',
        icon: 'bi-tags',
        apiPath: '/rental-order-labels',
        listPath: '/rental-order-labels',
        editPath: '/rental-order-labels',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'color', label: '顏色', type: 'color' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索標籤名稱...',
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true, fullWidth: true },
            { key: 'color', label: '顏色', type: 'color', required: true, fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: 'active', fullWidth: true}
        ]
    },

    // 付款方式管理
    'payment-methods': {
        title: '付款方式管理',
        icon: 'bi-credit-card',
        apiPath: '/payment-methods',
        listPath: '/payment-methods',
        editPath: '/payment-methods',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'code', label: '代碼', type: 'text' },
            { key: 'is_default', label: 'paymentMethods.defaultCustomer', preferLabel: true, type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: 'options.boolean.true', false: 'options.boolean.false' } },
            { key: 'is_default_expense', label: 'paymentMethods.defaultExpense', preferLabel: true, type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: 'options.boolean.true', false: 'options.boolean.false' } },
            { key: 'extra_fields.use_card_terminal', label: '使用卡機', type: 'badge', options: { true: 'info', false: 'secondary' }, labels: { true: '是', false: '否' } },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索付款方式名稱...',
        formFields: [
            { key: 'payment_type', label: '付款形式', type: 'select', required: true, fullWidth: true, options: [
                { value: 'normal', label: '普通' },
                { value: 'gateway', label: '線上閘道（手動輸入 API Key）' },
                { value: 'stripe_connect', label: 'Stripe Connect（推薦）', labelKey: 'pages.stripeConnect.paymentTypeStripeConnect' },
                { value: 'card_terminal', label: '卡機（Kpay/BBMSL/HSBC）' }
            ], defaultValue: 'normal'},
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'code', label: '代碼', type: 'text', required: true },
            { key: 'is_default', label: 'paymentMethods.defaultCustomer', preferLabel: true, type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'is_default_expense', label: 'paymentMethods.defaultExpense', preferLabel: true, type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'is_online_payment', label: '網店付款方式', type: 'select', required: false, fullWidth: true, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ], defaultValue: 'false'},
            // Gateway 連接字段（顯示/隱藏由 DynamicForm.handlePaymentTypeChange 控制）
            { key: 'stripe_api_key', label: 'Stripe API Key', type: 'text', required: false, fullWidth: true },
            { key: 'stripe_secret_key', label: 'Stripe Secret Key', type: 'password', required: false, fullWidth: true },
            { key: 'paypal_client_id', label: 'PayPal Client ID', type: 'text', required: false, fullWidth: true },
            { key: 'paypal_secret', label: 'PayPal Secret', type: 'password', required: false, fullWidth: true },
            { key: 'qfpay_app_code', label: 'QFPay App Code', type: 'text', required: false, fullWidth: true },
            { key: 'qfpay_client_key', label: 'QFPay Client Key', type: 'password', required: false, fullWidth: true },
            { key: 'qfpay_base_url', label: 'QFPay Base URL', type: 'text', required: false, fullWidth: true, placeholder: 'https://openapi-hk.qfapi.com' },
            { key: 'currency', label: 'Currency', type: 'select', required: false, fullWidth: true, options: [
                { value: 'hkd', label: 'HKD' },
                { value: 'cny', label: 'CNY' },
                { value: 'usd', label: 'USD' }
            ] },
            // 卡機連接字段（顯示/隱藏由 payment_type 控制）
            { key: 'extra_fields.use_card_terminal', label: '使用卡機付款', type: 'select', required: false, fullWidth: true, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ], defaultValue: 'false', showWhen: { field: 'payment_type', value: 'card_terminal' } },
            { key: 'extra_fields.card_terminal_id', label: '卡機 ID（在 vWork Connector 設定）', type: 'text', required: false, fullWidth: true, placeholder: '例如: terminal-1', showWhen: { field: 'payment_type', value: 'card_terminal' } },
            { key: 'extra_fields.card_terminal_type', label: '卡機類型', type: 'select', required: false, fullWidth: true, options: [
                { value: 'kpay', label: 'Kpay' },
                { value: 'bbmsl', label: 'BBMSL' },
                { value: 'hsbc', label: 'HSBC' }
            ], showWhen: { field: 'payment_type', value: 'card_terminal' } },
            // 狀態移到最底部，全寬
            { key: 'status', label: '狀態', type: 'select', required: false, fullWidth: true, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: 'active' }
        ]
    },

    // 運送方式管理
    'shipping-methods': {
        title: '運送方式管理',
        icon: 'bi-truck',
        apiPath: '/shipping-methods',
        listPath: '/shipping-methods',
        editPath: '/shipping-methods',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'code', label: '代碼', type: 'text' },
            { key: 'requires_shipping', label: '需要送貨', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } },
            { key: 'is_default', label: '系統預設', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } },
            { key: 'status', label: '狀態', type: 'badge', options: { default: 'primary', active: 'success' }, labels: { default: 'common.default', active: 'common.active' } }
        ],
        searchPlaceholder: '搜索運送方式名稱...',
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'code', label: '代碼', type: 'text', required: true },
            { key: 'requires_shipping', label: '需要送貨', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'is_default', label: '設為系統預設', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'status', label: '狀態', type: 'select', required: false, defaultValue: 'active', options: [
                { value: 'default', label: '預設' },
                { value: 'active', label: '活躍', labelKey: 'common.active' }
            ]}
        ]
    },

    // 物流公司管理
    'logistics-companies': {
        title: '物流公司管理',
        icon: 'bi-box-seam',
        apiPath: '/logistics-companies',
        listPath: '/logistics-companies',
        editPath: '/logistics-companies',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'code', label: '代碼', type: 'text' },
            { key: 'integration_type', label: '配送連接', type: 'badge', options: { none: 'secondary', sfexpress: 'primary', lalamove: 'success' }, labels: { none: '無', sfexpress: '順豐速遞', lalamove: '啦啦快送' } },
            { key: 'base_fee', label: '預設定額', type: 'currency' },
            { key: 'per_item_fee', label: '件價', type: 'currency' },
            { key: 'per_weight_fee', label: '重量價', type: 'currency' },
            { key: 'per_area_fee', label: '面積價', type: 'currency' },
            { key: 'is_default', label: '系統預設', type: 'badge', preferLabel: true, options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索物流公司名稱...',
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true },
            { key: 'code', label: '代碼', type: 'text', required: false },
            { key: 'integration_type', label: '配送連接', type: 'select', required: false, helpText: '選擇配送連接類型後，創建配送單時將自動調用對應 API 獲取物流單號。請先在「配送連接設定」中配置 API 憑證。', options: [
                { value: 'none', label: '無整合' },
                { value: 'sfexpress', label: '順豐速遞 SF Express' },
                { value: 'lalamove', label: '啦啦快送 Lalamove' }
            ], defaultValue: 'none' },
            { key: 'is_default', label: '系統預設', type: 'select', required: false, preferLabel: true, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'base_fee', label: '預設定額', type: 'number', required: false, step: '0.01' },
            { key: 'per_item_fee', label: '件價', type: 'number', required: false, step: '0.01' },
            { key: 'per_weight_fee', label: '重量價（每公斤）', type: 'number', required: false, step: '0.01' },
            { key: 'per_area_fee', label: '面積價（每平方米）', type: 'number', required: false, step: '0.01' },
            // 地區限制（存入 extra_fields）
            // 國家不輸入：全國家允許；地區不輸入：所選國家的所有地區允許
            { key: 'allowed_country_codes', label: '國家（可多選）', type: 'select2-multi', required: false, relationApi: '/countries', relationValueKey: 'code', relationLabelKey: 'name', relationLabelFields: ['name', 'code'], placeholder: '選擇國家（留空則允許所有國家）' },
            { key: 'allowed_region_keys', label: '地區（可多選）', type: 'select2-multi', required: false, relationApi: '/country-regions', relationValueKey: 'code', relationLabelKey: 'name', placeholder: '選擇地區（留空則允許所有地區）' },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: 'active'}
        ]
    },

    // 配送管理
    'shipments': {
        title: '配送管理',
        titleKey: 'shipmentsPage.title',
        icon: 'bi-box-seam',
        apiPath: '/shipments',
        listPath: '/shipments',
        editPath: '/shipments',
        exportEnabled: true,
        columns: [
            { key: 'shipment_number', label: '配送單號', labelKey: 'shipmentsPage.columns.shipmentNumber', type: 'text' },
            { key: 'tracking_number', label: '物流單號', labelKey: 'shipmentsPage.columns.trackingNumber', type: 'text' },
            { key: 'logistics_company.name', label: '物流公司', labelKey: 'shipmentsPage.columns.logisticsCompany', type: 'text' },
            { key: 'recipient_name', label: '收件人', labelKey: 'shipmentsPage.columns.recipientName', type: 'text' },
            { key: 'recipient_phone', label: '收件電話', labelKey: 'shipmentsPage.columns.recipientPhone', type: 'text' },
            { key: 'item_count', label: '件數', labelKey: 'shipmentsPage.columns.itemCount', type: 'number' },
            { key: 'total_fee', label: '費用', labelKey: 'shipmentsPage.columns.totalFee', type: 'currency' },
            { key: 'status', label: '狀態', labelKey: 'shipmentsPage.columns.status', type: 'badge', options: { 
                pending: 'secondary', 
                picked_up: 'info', 
                in_transit: 'primary', 
                out_for_delivery: 'warning',
                delivered: 'success', 
                failed: 'danger', 
                returned: 'dark',
                cancelled: 'secondary'
            }, labels: {
                pending: 'shipmentsPage.status.pending',
                picked_up: 'shipmentsPage.status.picked_up',
                in_transit: 'shipmentsPage.status.in_transit',
                out_for_delivery: 'shipmentsPage.status.out_for_delivery',
                delivered: 'shipmentsPage.status.delivered',
                failed: 'shipmentsPage.status.failed',
                returned: 'shipmentsPage.status.returned',
                cancelled: 'shipmentsPage.status.cancelled'
            }},
            { key: 'created_at', label: '建立時間', labelKey: 'shipmentsPage.columns.createdAt', type: 'datetime' }
        ],
        searchPlaceholder: '搜索配送單號、物流單號、收件人...',
        searchPlaceholderKey: 'shipmentsPage.searchPlaceholder',
        filters: [
            { key: 'status', label: '狀態', labelKey: 'shipmentsPage.filters.status', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'shipmentsPage.status.all' },
                { value: 'pending', label: '待處理', labelKey: 'shipmentsPage.status.pending' },
                { value: 'picked_up', label: '已取件', labelKey: 'shipmentsPage.status.picked_up' },
                { value: 'in_transit', label: '運送中', labelKey: 'shipmentsPage.status.in_transit' },
                { value: 'out_for_delivery', label: '派送中', labelKey: 'shipmentsPage.status.out_for_delivery' },
                { value: 'delivered', label: '已送達', labelKey: 'shipmentsPage.status.delivered' },
                { value: 'failed', label: '配送失敗', labelKey: 'shipmentsPage.status.failed' },
                { value: 'returned', label: '已退回', labelKey: 'shipmentsPage.status.returned' },
                { value: 'cancelled', label: '已取消', labelKey: 'shipmentsPage.status.cancelled' }
            ]},
            { key: 'logistics_company_id', label: '物流公司', labelKey: 'shipmentsPage.filters.logisticsCompany', type: 'select2', relationApi: '/logistics-companies', relationValueKey: 'id', relationLabelKey: 'name' }
        ],
        formFields: [
            { key: 'logistics_company_id', label: '物流公司', labelKey: 'shipmentsPage.fields.logisticsCompanyId', type: 'select2', required: false, relationApi: '/logistics-companies', relationValueKey: 'id', relationLabelKey: 'name', placeholder: '選擇物流公司', placeholderKey: 'shipmentsPage.logisticsCompanyPlaceholder' },
            { key: 'tracking_number', label: '物流單號', labelKey: 'shipmentsPage.fields.trackingNumber', type: 'text', required: false },
            { key: 'sender_name', label: '發件人姓名', labelKey: 'shipmentsPage.fields.senderName', type: 'text', required: false },
            { key: 'sender_phone', label: '發件人電話', labelKey: 'shipmentsPage.fields.senderPhone', type: 'text', required: false },
            { key: 'sender_address', label: '發件人地址', labelKey: 'shipmentsPage.fields.senderAddress', type: 'textarea', required: false },
            { key: 'recipient_name', label: '收件人姓名', labelKey: 'shipmentsPage.fields.recipientName', type: 'text', required: true },
            { key: 'recipient_phone', label: '收件人電話', labelKey: 'shipmentsPage.fields.recipientPhone', type: 'text', required: true },
            { key: 'recipient_address', label: '收件人地址', labelKey: 'shipmentsPage.fields.recipientAddress', type: 'textarea', required: true },
            { key: 'items', label: '產品明細', labelKey: 'shipmentsPage.fields.items', type: 'shipment-items', required: false, fullWidth: true, helpText: '參考訂單發貨單/退款單的產品明細', helpTextKey: 'shipmentsPage.fields.itemsHelpText' },
            { key: 'weight', label: '重量（公斤）', labelKey: 'shipmentsPage.fields.weight', type: 'number', required: false, step: '0.001' },
            { key: 'dimensions', label: '尺寸（長x寬x高）', labelKey: 'shipmentsPage.fields.dimensions', type: 'text', required: false, placeholder: '例：30x20x15 cm', placeholderKey: 'shipmentsPage.fields.dimensionsPlaceholder' },
            { key: 'description', label: '配送內容', labelKey: 'shipmentsPage.fields.description', type: 'textarea', required: false },
            { key: 'shipping_fee', label: '運費', labelKey: 'shipmentsPage.fields.shippingFee', type: 'number', required: false, step: '0.01' },
            { key: 'insurance_fee', label: '保險費', labelKey: 'shipmentsPage.fields.insuranceFee', type: 'number', required: false, step: '0.01' },
            { key: 'notes', label: '備註', labelKey: 'shipmentsPage.fields.notes', type: 'textarea', required: false }
        ]
    },

    // 銀行賬戶管理
    'bank-accounts': {
        title: '銀行賬戶管理',
        icon: 'bi-bank',
        apiPath: '/bank-accounts',
        listPath: '/bank-accounts',
        editPath: '/bank-accounts',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '賬戶名稱', type: 'text' },
            { key: 'bank_name', label: '銀行名稱', type: 'text' },
            { key: 'account_number', label: '賬戶號碼', type: 'text' },
            { key: 'account_holder', label: '戶名', type: 'text' },
            { key: 'currency', label: '幣種', type: 'text' },
            { key: 'is_default_receiving', label: '系統預設收款帳號', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } },
            { key: 'is_default_payment', label: '系統預設付款帳號', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' }, labels: { active: '啟用', inactive: '停用' } }
        ],
        searchPlaceholder: '搜索銀行賬戶名稱或銀行名稱...',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'active', label: '啟用', labelKey: 'common.active' },
                { value: 'inactive', label: '停用', labelKey: 'common.inactive' }
            ]}
        ],
        formFields: [
            { key: 'name', label: '賬戶名稱', type: 'text', required: true, placeholder: '例如：中國銀行主賬戶' },
            { key: 'bank_name', label: '銀行名稱', type: 'text', required: true, placeholder: '例如：中國銀行' },
            { key: 'account_number', label: '賬戶號碼', type: 'text', required: true, placeholder: '例如：1234567890' },
            { key: 'account_holder', label: '戶名', type: 'text', required: false, placeholder: '賬戶持有人姓名' },
            { key: 'currency', label: '貨幣', type: 'select2', relationApi: '/currencies', relationValueKey: 'code', relationLabelKey: 'code', relationDisplayFormat: 'code-name', required: false, placeholder: '請選擇貨幣...' },
            { key: 'is_default_receiving', label: '設為系統預設收款帳號', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'is_default_payment', label: '設為系統預設付款帳號', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]},
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '啟用', labelKey: 'common.active' },
                { value: 'inactive', label: '停用', labelKey: 'common.inactive' }
            ], defaultValue: 'active' },
            { key: 'notes', label: '備註', type: 'textarea', required: false, placeholder: '可選的備註信息' }
        ]
    },

    // 電話區號管理
    'phone-country-codes': {
        title: '電話區號',
        icon: 'bi-telephone',
        apiPath: '/phone-country-codes',
        listPath: '/phone-country-codes',
        editPath: '/phone-country-codes',
        exportEnabled: false,
        columns: [
            { key: 'code', label: '區號', type: 'text' },
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'is_default', label: '系統預設', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } }
        ],
        searchPlaceholder: '搜索區號或名稱...',
        formFields: [
            { key: 'code', label: '區號', type: 'select2', required: true, options: Object.keys(COUNTRY_PHONE_CODES).sort().map(code => ({ value: code, label: code })) },
            { key: 'name', label: '名稱', type: 'text', required: true, readonly: true, placeholder: '選擇區號後自動填充' },
            { key: 'is_default', label: '設為系統預設', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ]}
        ]
    },

    // 採購標籤管理
    'purchase-order-labels': {
        title: '採購標籤管理',
        icon: 'bi-tags',
        apiPath: '/purchase-order-labels',
        listPath: '/purchase-order-labels',
        editPath: '/purchase-order-labels',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text' },
            { key: 'color', label: '顏色', type: 'color' },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', inactive: 'secondary' } }
        ],
        searchPlaceholder: '搜索標籤名稱...',
        formFields: [
            { key: 'name', label: '名稱', type: 'text', required: true, fullWidth: true },
            { key: 'color', label: '顏色', type: 'color', required: false, defaultValue: '#007bff', fullWidth: true },
            { key: 'status', label: '狀態', type: 'select', required: false, options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ], defaultValue: 'active', fullWidth: true }
        ]
    },
    blogs: {
        apiPath: '/blogs',
        listPath: '/blogs',
        editPath: '/blogs',
        listTitle: '博客管理',
        newTitle: '新增博客',
        editTitle: '編輯博客',
        columns: [
            { key: 'title', label: '標題', type: 'text' },
            { key: 'slug', label: '網址', type: 'text' },
            { key: 'category', label: '分類', type: 'text' },
            { key: 'status', label: '狀態', type: 'badge', options: { draft: 'secondary', published: 'success', archived: 'dark' }, labels: { draft: '草稿', published: '已發布', archived: '已歸檔' } },
            { key: 'published_at', label: '發布時間', type: 'datetime' },
            { key: 'view_count', label: '瀏覽數', type: 'number' }
        ],
        formFields: [
            { key: 'title', label: '標題', type: 'text', required: true },
            { key: 'slug', label: '網址', type: 'text', required: false },
            { key: 'featured_image', label: '特色圖片', type: 'file', required: false, accept: 'image/*' },
            { key: 'content', label: '內容', type: 'html-editor', required: false, fullWidth: true },
            { key: 'excerpt', label: '摘要', type: 'textarea', required: false, fullWidth: true },
            { key: 'category', label: '分類', type: 'text', required: false },
            { key: 'status', label: '狀態', type: 'select', required: true, options: [
                { value: 'draft', label: '草稿' },
                { value: 'published', label: '已發布' },
                { value: 'archived', label: '已歸檔' }
            ], defaultValue: 'draft' },
            { key: 'published_at', label: '發布時間', type: 'datetime', required: false },
            { key: 'seo_title', label: 'SEO 標題', type: 'text', required: false, fullWidth: true },
            { key: 'seo_description', label: 'SEO 描述', type: 'textarea', required: false, fullWidth: true },
            { key: 'seo_keywords', label: 'SEO 關鍵字', type: 'text', required: false, fullWidth: true }
        ]
    },

    // 工作時段
    shifts: {
        title: '工作時段',
        icon: 'bi-clock',
        apiPath: '/shifts',
        listPath: '/shifts',
        editPath: '/shifts',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '時段名稱', type: 'text' },
            { key: 'start_time', label: '上班時間', type: 'time' },
            { key: 'end_time', label: '下班時間', type: 'time' },
            { key: 'is_default', label: '預設', type: 'badge', options: { true: 'success', false: 'secondary' }, labels: { true: '是', false: '否' } }
        ],
        formFields: [
            { key: 'name', label: '時段名稱', type: 'text', required: true, placeholder: '例如：早班、中班、晚班', fullWidth: true },
            { key: 'start_time', label: '上班時間', type: 'time', required: true, defaultValue: '09:00', fullWidth: true },
            { key: 'end_time', label: '下班時間', type: 'time', required: true, defaultValue: '18:00', fullWidth: true },
            { key: 'is_default', label: '系統預設', type: 'select', required: false, options: [
                { value: 'true', label: '是' },
                { value: 'false', label: '否' }
            ], defaultValue: 'false', fullWidth: true }
        ]
    },

    // 業務目標
    'business_goals': {
        title: '業務目標',
        icon: 'bi-bullseye',
        apiPath: '/business-goals',
        listPath: '/business-goals',
        editPath: '/business-goals',
        exportEnabled: false,
        columns: [
            { key: 'title', label: '目標名稱', type: 'text' },
            { key: 'metric_type', label: '指標類型', type: 'badge', options: {
                order_count: 'primary', revenue: 'success', customer_count: 'info',
                product_sales_qty: 'warning', service_order_count: 'secondary', custom: 'dark'
            }},
            { key: 'target_value', label: '目標值', type: 'number' },
            { key: 'current_value', label: '當前值', type: 'number' },
            { key: 'end_date', label: '截止日期', type: 'date' },
            { key: 'priority', label: '優先級', type: 'badge', options: { high: 'danger', medium: 'warning', low: 'secondary' } },
            { key: 'status', label: '狀態', type: 'badge', options: { active: 'success', completed: 'primary', failed: 'danger', paused: 'secondary' } }
        ],
        searchPlaceholder: '搜索業務目標...',
        searchPlaceholderKey: 'businessGoals.searchPlaceholder',
        filters: [
            { key: 'status', label: '狀態', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'active', label: '進行中', labelKey: 'businessGoals.formStatusActive' },
                { value: 'completed', label: '已達成', labelKey: 'businessGoals.formStatusCompleted' },
                { value: 'failed', label: '未達成', labelKey: 'businessGoals.formStatusFailed' },
                { value: 'paused', label: '已暫停', labelKey: 'businessGoals.formStatusPaused' }
            ]},
            { key: 'priority', label: '優先級', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'high', label: '高', labelKey: 'businessGoals.formPriorityHigh' },
                { value: 'medium', label: '中', labelKey: 'businessGoals.formPriorityMedium' },
                { value: 'low', label: '低', labelKey: 'businessGoals.formPriorityLow' }
            ]},
            { key: 'metric_type', label: '指標類型', type: 'select', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'order_count', label: '訂單數量', labelKey: 'businessGoals.metricOrderCount' },
                { value: 'revenue', label: '營業額', labelKey: 'businessGoals.metricRevenue' },
                { value: 'customer_count', label: '客戶數量', labelKey: 'businessGoals.metricCustomerCount' },
                { value: 'product_sales_qty', label: '產品銷量', labelKey: 'businessGoals.metricProductSales' },
                { value: 'service_order_count', label: '服務訂單數', labelKey: 'businessGoals.metricServiceOrders' },
                { value: 'custom', label: '自定義', labelKey: 'businessGoals.metricCustom' }
            ]}
        ],
        formFields: [
            { key: 'metric_type', label: '指標類型', type: 'select', required: true, defaultValue: 'custom', options: [
                { value: 'order_count', label: '訂單數量（自動追蹤）', labelKey: 'businessGoals.formMetricOrderCount' },
                { value: 'revenue', label: '營業額（自動追蹤）', labelKey: 'businessGoals.formMetricRevenue' },
                { value: 'customer_count', label: '新客戶數量（自動追蹤）', labelKey: 'businessGoals.formMetricCustomerCount' },
                { value: 'product_sales_qty', label: '產品銷量（自動追蹤）', labelKey: 'businessGoals.formMetricProductSales' },
                { value: 'service_order_count', label: '服務訂單數（自動追蹤）', labelKey: 'businessGoals.formMetricServiceOrders' },
                { value: 'custom', label: '自定義（手動更新）', labelKey: 'businessGoals.formMetricCustom' }
            ]},
            { key: 'target_value', label: '目標值', type: 'number', required: true, placeholder: '例如：100' },
            { key: 'title', label: '目標名稱', type: 'text', required: false, fullWidth: true, placeholder: '留空將根據指標類型和目標值自動生成' },
            { key: 'description', label: '目標描述', type: 'textarea', required: false, fullWidth: true, placeholder: '詳細說明目標內容和背景' },
            { key: 'start_date', label: '開始日期', type: 'date', required: true },
            { key: 'end_date', label: '截止日期', type: 'date', required: true },
            { key: 'status', label: '狀態', type: 'select', required: false, defaultValue: 'active', options: [
                { value: 'active', label: '進行中', labelKey: 'businessGoals.formStatusActive' },
                { value: 'completed', label: '已達成', labelKey: 'businessGoals.formStatusCompleted' },
                { value: 'failed', label: '未達成', labelKey: 'businessGoals.formStatusFailed' },
                { value: 'paused', label: '已暫停', labelKey: 'businessGoals.formStatusPaused' }
            ]},
            { key: 'priority', label: '優先級', type: 'button-group', required: false, defaultValue: 'medium', options: [
                { value: 'high', label: '高', labelKey: 'businessGoals.formPriorityHigh' },
                { value: 'medium', label: '中', labelKey: 'businessGoals.formPriorityMedium' },
                { value: 'low', label: '低', labelKey: 'businessGoals.formPriorityLow' }
            ]}
        ]
    },

    // 項目類型
    'project_types': {
        title: '項目類型管理',
        titleKey: 'projectTypes.title',
        icon: 'bi-folder2',
        apiPath: '/project-types',
        listPath: '/project-types',
        editPath: '/project-types',
        exportEnabled: false,
        columns: [
            { key: 'name', label: '名稱', type: 'text', labelKey: 'common.name' },
            { key: 'color', label: '顏色', type: 'color' },
            { key: 'status', label: '狀態', type: 'badge', labelKey: 'common.status', options: { active: 'success', inactive: 'secondary' } }
        ],
        filters: [
            { key: 'status', label: '狀態', type: 'select', labelKey: 'common.status', options: [
                { value: '', label: '全部', labelKey: 'common.all' },
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ]}
        ],
        formFields: [
            { key: 'name', label: '類型名稱', type: 'text', required: true, placeholder: '例如：軟體開發、市場推廣', fullWidth: true },
            { key: 'color', label: '顏色', type: 'color', required: false, defaultValue: '#8b5cf6' },
            { key: 'status', label: '狀態', type: 'select', required: false, defaultValue: 'active', options: [
                { value: 'active', label: '活躍', labelKey: 'common.active' },
                { value: 'inactive', label: '非活躍', labelKey: 'common.inactive' }
            ]}
        ]
    }
};

// 兼容 URL 使用連字符的 pageName（如 /project-types）
if (PageConfigs['project_types'] && !PageConfigs['project-types']) {
    PageConfigs['project-types'] = PageConfigs['project_types'];
}
if (PageConfigs['project-types'] && !PageConfigs['project_types']) {
    PageConfigs['project_types'] = PageConfigs['project-types'];
}

// HR：兼容 /job-vacancies /job-hires
if (PageConfigs['job_vacancies'] && !PageConfigs['job-vacancies']) {
    PageConfigs['job-vacancies'] = PageConfigs['job_vacancies'];
}
if (PageConfigs['job-vacancies'] && !PageConfigs['job_vacancies']) {
    PageConfigs['job_vacancies'] = PageConfigs['job-vacancies'];
}
if (PageConfigs['job_hires'] && !PageConfigs['job-hires']) {
    PageConfigs['job-hires'] = PageConfigs['job_hires'];
}
if (PageConfigs['job-hires'] && !PageConfigs['job_hires']) {
    PageConfigs['job_hires'] = PageConfigs['job-hires'];
}

// HR：兼容 /job-applicants
if (PageConfigs['job_applicants'] && !PageConfigs['job-applicants']) {
    PageConfigs['job-applicants'] = PageConfigs['job_applicants'];
}
if (PageConfigs['job-applicants'] && !PageConfigs['job_applicants']) {
    PageConfigs['job_applicants'] = PageConfigs['job-applicants'];
}

// 薪資附加項目 presets：兼容 /payroll-adjustment-presets
if (PageConfigs['payroll_adjustment_presets'] && !PageConfigs['payroll-adjustment-presets']) {
    PageConfigs['payroll-adjustment-presets'] = PageConfigs['payroll_adjustment_presets'];
}
if (PageConfigs['payroll-adjustment-presets'] && !PageConfigs['payroll_adjustment_presets']) {
    PageConfigs['payroll_adjustment_presets'] = PageConfigs['payroll-adjustment-presets'];
}

// 業務目標：兼容 /business-goals
if (PageConfigs['business_goals'] && !PageConfigs['business-goals']) {
    PageConfigs['business-goals'] = PageConfigs['business_goals'];
}
if (PageConfigs['business-goals'] && !PageConfigs['business_goals']) {
    PageConfigs['business_goals'] = PageConfigs['business-goals'];
}

// 獲取配置（返回深拷貝，防止 SPA 導航時 config 被永久修改）
function getPageConfig(pageName) {
    const config = PageConfigs[pageName];
    if (!config) return null;
    // Deep copy to prevent mutations (e.g. prepareVMarketProductFields filter)
    // from permanently altering the global PageConfigs across SPA navigations
    try {
        return JSON.parse(JSON.stringify(config));
    } catch (e) {
        console.warn('getPageConfig: deep copy failed, returning shallow copy', e);
        return Object.assign({}, config, {
            formFields: config.formFields ? config.formFields.map(f => Object.assign({}, f)) : [],
            tableColumns: config.tableColumns ? config.tableColumns.map(c => Object.assign({}, c)) : []
        });
    }
}

// 獲取所有配置的頁面名稱
function getAllPageNames() {
    return Object.keys(PageConfigs);
}
