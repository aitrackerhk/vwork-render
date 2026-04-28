package utils

import (
	"log"
	"nwork/internal/database"
	"nwork/internal/models"
	"time"

	"github.com/google/uuid"
)

// PhoneCodeToCurrency 电话区码到货币代码的映射
var PhoneCodeToCurrency = map[string]string{
	"+1":   "USD", // United States / Canada
	"+7":   "RUB", // Russia / Kazakhstan
	"+20":  "EGP", // Egypt
	"+27":  "ZAR", // South Africa
	"+30":  "EUR", // Greece
	"+31":  "EUR", // Netherlands
	"+32":  "EUR", // Belgium
	"+33":  "EUR", // France
	"+34":  "EUR", // Spain
	"+36":  "HUF", // Hungary
	"+39":  "EUR", // Italy
	"+40":  "RON", // Romania
	"+41":  "CHF", // Switzerland
	"+43":  "EUR", // Austria
	"+44":  "GBP", // United Kingdom
	"+45":  "DKK", // Denmark
	"+46":  "SEK", // Sweden
	"+47":  "NOK", // Norway
	"+48":  "PLN", // Poland
	"+49":  "EUR", // Germany
	"+51":  "PEN", // Peru
	"+52":  "MXN", // Mexico
	"+53":  "CUP", // Cuba
	"+54":  "ARS", // Argentina
	"+55":  "BRL", // Brazil
	"+56":  "CLP", // Chile
	"+57":  "COP", // Colombia
	"+58":  "VES", // Venezuela
	"+60":  "MYR", // Malaysia
	"+61":  "AUD", // Australia
	"+62":  "IDR", // Indonesia
	"+63":  "PHP", // Philippines
	"+64":  "NZD", // New Zealand
	"+65":  "SGD", // Singapore
	"+66":  "THB", // Thailand
	"+81":  "JPY", // Japan
	"+82":  "KRW", // South Korea
	"+84":  "VND", // Vietnam
	"+86":  "CNY", // China
	"+90":  "TRY", // Turkey
	"+91":  "INR", // India
	"+92":  "PKR", // Pakistan
	"+93":  "AFN", // Afghanistan
	"+94":  "LKR", // Sri Lanka
	"+95":  "MMK", // Myanmar
	"+98":  "IRR", // Iran
	"+212": "MAD", // Morocco
	"+213": "DZD", // Algeria
	"+216": "TND", // Tunisia
	"+218": "LYD", // Libya
	"+220": "GMD", // Gambia
	"+221": "XOF", // Senegal
	"+222": "MRU", // Mauritania
	"+223": "XOF", // Mali
	"+224": "GNF", // Guinea
	"+225": "XOF", // Ivory Coast
	"+226": "XOF", // Burkina Faso
	"+227": "XOF", // Niger
	"+228": "XOF", // Togo
	"+229": "XOF", // Benin
	"+230": "MUR", // Mauritius
	"+231": "LRD", // Liberia
	"+232": "SLE", // Sierra Leone
	"+233": "GHS", // Ghana
	"+234": "NGN", // Nigeria
	"+235": "XAF", // Chad
	"+236": "XAF", // Central African Republic
	"+237": "XAF", // Cameroon
	"+238": "CVE", // Cape Verde
	"+239": "STN", // São Tomé and Príncipe
	"+240": "XAF", // Equatorial Guinea
	"+241": "XAF", // Gabon
	"+242": "XAF", // Republic of the Congo
	"+243": "CDF", // Democratic Republic of the Congo
	"+244": "AOA", // Angola
	"+245": "XOF", // Guinea-Bissau
	"+246": "USD", // British Indian Ocean Territory
	"+248": "SCR", // Seychelles
	"+249": "SDG", // Sudan
	"+250": "RWF", // Rwanda
	"+251": "ETB", // Ethiopia
	"+252": "SOS", // Somalia
	"+253": "DJF", // Djibouti
	"+254": "KES", // Kenya
	"+255": "TZS", // Tanzania
	"+256": "UGX", // Uganda
	"+257": "BIF", // Burundi
	"+258": "MZN", // Mozambique
	"+260": "ZMW", // Zambia
	"+261": "MGA", // Madagascar
	"+262": "EUR", // Réunion / Mayotte
	"+263": "ZWL", // Zimbabwe
	"+264": "NAD", // Namibia
	"+265": "MWK", // Malawi
	"+266": "LSL", // Lesotho
	"+267": "BWP", // Botswana
	"+268": "SZL", // Eswatini
	"+269": "KMF", // Comoros
	"+290": "SHP", // Saint Helena
	"+291": "ERN", // Eritrea
	"+297": "AWG", // Aruba
	"+298": "DKK", // Faroe Islands
	"+299": "DKK", // Greenland
	"+350": "GIP", // Gibraltar
	"+351": "EUR", // Portugal
	"+352": "EUR", // Luxembourg
	"+353": "EUR", // Ireland
	"+354": "ISK", // Iceland
	"+355": "ALL", // Albania
	"+356": "EUR", // Malta
	"+357": "EUR", // Cyprus
	"+358": "EUR", // Finland
	"+359": "BGN", // Bulgaria
	"+370": "EUR", // Lithuania
	"+371": "EUR", // Latvia
	"+372": "EUR", // Estonia
	"+373": "MDL", // Moldova
	"+374": "AMD", // Armenia
	"+375": "BYN", // Belarus
	"+376": "EUR", // Andorra
	"+377": "EUR", // Monaco
	"+378": "EUR", // San Marino
	"+380": "UAH", // Ukraine
	"+381": "RSD", // Serbia
	"+382": "EUR", // Montenegro
	"+383": "EUR", // Kosovo
	"+385": "EUR", // Croatia
	"+386": "EUR", // Slovenia
	"+387": "BAM", // Bosnia and Herzegovina
	"+389": "MKD", // North Macedonia
	"+420": "CZK", // Czech Republic
	"+421": "EUR", // Slovakia
	"+423": "CHF", // Liechtenstein
	"+500": "FKP", // Falkland Islands
	"+501": "BZD", // Belize
	"+502": "GTQ", // Guatemala
	"+503": "USD", // El Salvador
	"+504": "HNL", // Honduras
	"+505": "NIO", // Nicaragua
	"+506": "CRC", // Costa Rica
	"+507": "PAB", // Panama
	"+508": "EUR", // Saint Pierre and Miquelon
	"+509": "HTG", // Haiti
	"+590": "EUR", // Guadeloupe
	"+591": "BOB", // Bolivia
	"+592": "GYD", // Guyana
	"+593": "USD", // Ecuador
	"+594": "EUR", // French Guiana
	"+595": "PYG", // Paraguay
	"+596": "EUR", // Martinique
	"+597": "SRD", // Suriname
	"+598": "UYU", // Uruguay
	"+599": "ANG", // Netherlands Antilles
	"+670": "USD", // East Timor
	"+672": "AUD", // Australian External Territories
	"+673": "BND", // Brunei
	"+674": "AUD", // Nauru
	"+675": "PGK", // Papua New Guinea
	"+676": "TOP", // Tonga
	"+677": "SBD", // Solomon Islands
	"+678": "VUV", // Vanuatu
	"+679": "FJD", // Fiji
	"+680": "USD", // Palau
	"+681": "XPF", // Wallis and Futuna
	"+682": "NZD", // Cook Islands
	"+683": "NZD", // Niue
	"+685": "WST", // Samoa
	"+686": "AUD", // Kiribati
	"+687": "XPF", // New Caledonia
	"+688": "AUD", // Tuvalu
	"+689": "XPF", // French Polynesia
	"+690": "NZD", // Tokelau
	"+691": "USD", // Micronesia
	"+692": "USD", // Marshall Islands
	"+850": "KPW", // North Korea
	"+852": "HKD", // Hong Kong
	"+853": "MOP", // Macau
	"+855": "KHR", // Cambodia
	"+856": "LAK", // Laos
	"+880": "BDT", // Bangladesh
	"+886": "TWD", // Taiwan
	"+960": "MVR", // Maldives
	"+961": "LBP", // Lebanon
	"+962": "JOD", // Jordan
	"+963": "SYP", // Syria
	"+964": "IQD", // Iraq
	"+965": "KWD", // Kuwait
	"+966": "SAR", // Saudi Arabia
	"+967": "YER", // Yemen
	"+968": "OMR", // Oman
	"+970": "ILS", // Palestine
	"+971": "AED", // United Arab Emirates
	"+972": "ILS", // Israel
	"+973": "BHD", // Bahrain
	"+974": "QAR", // Qatar
	"+975": "BTN", // Bhutan
	"+976": "MNT", // Mongolia
	"+977": "NPR", // Nepal
	"+992": "TJS", // Tajikistan
	"+993": "TMT", // Turkmenistan
	"+994": "AZN", // Azerbaijan
	"+995": "GEL", // Georgia
	"+996": "KGS", // Kyrgyzstan
	"+998": "UZS", // Uzbekistan
}

// CurrencyInfo 货币信息
type CurrencyInfo struct {
	Code   string
	Name   string
	Symbol string
}

// CurrencyData 货币数据映射
var CurrencyData = map[string]CurrencyInfo{
	"USD": {"USD", "US Dollar", "$"},
	"EUR": {"EUR", "Euro", "€"},
	"GBP": {"GBP", "British Pound", "£"},
	"JPY": {"JPY", "Japanese Yen", "¥"},
	"CNY": {"CNY", "Chinese Yuan", "¥"},
	"HKD": {"HKD", "Hong Kong Dollar", "HK$"},
	"SGD": {"SGD", "Singapore Dollar", "S$"},
	"KRW": {"KRW", "South Korean Won", "₩"},
	"TWD": {"TWD", "New Taiwan Dollar", "NT$"},
	"THB": {"THB", "Thai Baht", "฿"},
	"MYR": {"MYR", "Malaysian Ringgit", "RM"},
	"IDR": {"IDR", "Indonesian Rupiah", "Rp"},
	"PHP": {"PHP", "Philippine Peso", "₱"},
	"VND": {"VND", "Vietnamese Dong", "₫"},
	"INR": {"INR", "Indian Rupee", "₹"},
	"AUD": {"AUD", "Australian Dollar", "A$"},
	"NZD": {"NZD", "New Zealand Dollar", "NZ$"},
	"CAD": {"CAD", "Canadian Dollar", "C$"},
	"CHF": {"CHF", "Swiss Franc", "CHF"},
	"RUB": {"RUB", "Russian Ruble", "₽"},
	"BRL": {"BRL", "Brazilian Real", "R$"},
	"MXN": {"MXN", "Mexican Peso", "Mex$"},
	"ARS": {"ARS", "Argentine Peso", "$"},
	"CLP": {"CLP", "Chilean Peso", "$"},
	"COP": {"COP", "Colombian Peso", "$"},
	"PEN": {"PEN", "Peruvian Sol", "S/"},
	"MOP": {"MOP", "Macanese Pataca", "MOP$"},
	"ZAR": {"ZAR", "South African Rand", "R"},
	"EGP": {"EGP", "Egyptian Pound", "E£"},
	"TRY": {"TRY", "Turkish Lira", "₺"},
	"PKR": {"PKR", "Pakistani Rupee", "₨"},
	"BDT": {"BDT", "Bangladeshi Taka", "৳"},
	"LKR": {"LKR", "Sri Lankan Rupee", "Rs"},
	"NPR": {"NPR", "Nepalese Rupee", "Rs"},
	"MMK": {"MMK", "Myanmar Kyat", "K"},
	"KHR": {"KHR", "Cambodian Riel", "៛"},
	"LAK": {"LAK", "Lao Kip", "₭"},
	"AFN": {"AFN", "Afghan Afghani", "؋"},
	"IRR": {"IRR", "Iranian Rial", "﷼"},
	"IQD": {"IQD", "Iraqi Dinar", "ع.د"},
	"JOD": {"JOD", "Jordanian Dinar", "د.ا"},
	"KWD": {"KWD", "Kuwaiti Dinar", "د.ك"},
	"BHD": {"BHD", "Bahraini Dinar", ".د.ب"},
	"OMR": {"OMR", "Omani Rial", "﷼"},
	"QAR": {"QAR", "Qatari Riyal", "﷼"},
	"SAR": {"SAR", "Saudi Riyal", "﷼"},
	"AED": {"AED", "UAE Dirham", "د.إ"},
	"ILS": {"ILS", "Israeli Shekel", "₪"},
	"YER": {"YER", "Yemeni Rial", "﷼"},
	"SYP": {"SYP", "Syrian Pound", "£"},
	"LBP": {"LBP", "Lebanese Pound", "£"},
	"MVR": {"MVR", "Maldivian Rufiyaa", "Rf"},
	"BTN": {"BTN", "Bhutanese Ngultrum", "Nu."},
	"MNT": {"MNT", "Mongolian Tugrik", "₮"},
	"TJS": {"TJS", "Tajikistani Somoni", "SM"},
	"TMT": {"TMT", "Turkmenistani Manat", "m"},
	"AZN": {"AZN", "Azerbaijani Manat", "₼"},
	"GEL": {"GEL", "Georgian Lari", "₾"},
	"KGS": {"KGS", "Kyrgyzstani Som", "лв"},
	"UZS": {"UZS", "Uzbekistani Som", "лв"},
	"AMD": {"AMD", "Armenian Dram", "֏"},
	"BYN": {"BYN", "Belarusian Ruble", "Br"},
	"MDL": {"MDL", "Moldovan Leu", "lei"},
	"UAH": {"UAH", "Ukrainian Hryvnia", "₴"},
	"RSD": {"RSD", "Serbian Dinar", "дин."},
	"BAM": {"BAM", "Bosnia-Herzegovina Convertible Mark", "KM"},
	"MKD": {"MKD", "Macedonian Denar", "ден"},
	"CZK": {"CZK", "Czech Koruna", "Kč"},
	"PLN": {"PLN", "Polish Zloty", "zł"},
	"HUF": {"HUF", "Hungarian Forint", "Ft"},
	"RON": {"RON", "Romanian Leu", "lei"},
	"BGN": {"BGN", "Bulgarian Lev", "лв"},
	"DKK": {"DKK", "Danish Krone", "kr"},
	"SEK": {"SEK", "Swedish Krona", "kr"},
	"NOK": {"NOK", "Norwegian Krone", "kr"},
	"ISK": {"ISK", "Icelandic Krona", "kr"},
	"ALL": {"ALL", "Albanian Lek", "L"},
	"XOF": {"XOF", "West African CFA Franc", "CFA"},
	"XAF": {"XAF", "Central African CFA Franc", "CFA"},
	"XPF": {"XPF", "CFP Franc", "₣"},
	"NGN": {"NGN", "Nigerian Naira", "₦"},
	"GHS": {"GHS", "Ghanaian Cedi", "₵"},
	"KES": {"KES", "Kenyan Shilling", "KSh"},
	"UGX": {"UGX", "Ugandan Shilling", "USh"},
	"TZS": {"TZS", "Tanzanian Shilling", "TSh"},
	"ETB": {"ETB", "Ethiopian Birr", "Br"},
	"RWF": {"RWF", "Rwandan Franc", "RF"},
	"BIF": {"BIF", "Burundian Franc", "FBu"},
	"DJF": {"DJF", "Djiboutian Franc", "Fdj"},
	"SOS": {"SOS", "Somali Shilling", "S"},
	"SDG": {"SDG", "Sudanese Pound", "ج.س."},
	"ERN": {"ERN", "Eritrean Nakfa", "Nfk"},
	"MZN": {"MZN", "Mozambican Metical", "MT"},
	"ZMW": {"ZMW", "Zambian Kwacha", "ZK"},
	"MWK": {"MWK", "Malawian Kwacha", "MK"},
	"ZWL": {"ZWL", "Zimbabwean Dollar", "Z$"},
	"NAD": {"NAD", "Namibian Dollar", "N$"},
	"BWP": {"BWP", "Botswana Pula", "P"},
	"SZL": {"SZL", "Eswatini Lilangeni", "L"},
	"LSL": {"LSL", "Lesotho Loti", "L"},
	"MGA": {"MGA", "Malagasy Ariary", "Ar"},
	"KMF": {"KMF", "Comorian Franc", "CF"},
	"MUR": {"MUR", "Mauritian Rupee", "Rs"},
	"SCR": {"SCR", "Seychellois Rupee", "SR"},
	"LRD": {"LRD", "Liberian Dollar", "$"},
	"SLE": {"SLE", "Sierra Leonean Leone", "Le"},
	"CVE": {"CVE", "Cape Verdean Escudo", "Esc"},
	"STN": {"STN", "São Tomé and Príncipe Dobra", "Db"},
	"CDF": {"CDF", "Congolese Franc", "FC"},
	"AOA": {"AOA", "Angolan Kwanza", "Kz"},
	"MRU": {"MRU", "Mauritanian Ouguiya", "UM"},
	"GNF": {"GNF", "Guinean Franc", "FG"},
	"LYD": {"LYD", "Libyan Dinar", "ل.د"},
	"TND": {"TND", "Tunisian Dinar", "د.ت"},
	"DZD": {"DZD", "Algerian Dinar", "د.ج"},
	"MAD": {"MAD", "Moroccan Dirham", "د.م."},
	"HTG": {"HTG", "Haitian Gourde", "G"},
	"GTQ": {"GTQ", "Guatemalan Quetzal", "Q"},
	"HNL": {"HNL", "Honduran Lempira", "L"},
	"NIO": {"NIO", "Nicaraguan Córdoba", "C$"},
	"CRC": {"CRC", "Costa Rican Colón", "₡"},
	"PAB": {"PAB", "Panamanian Balboa", "B/."},
	"BOB": {"BOB", "Bolivian Boliviano", "Bs."},
	"GYD": {"GYD", "Guyanese Dollar", "$"},
	"PYG": {"PYG", "Paraguayan Guaraní", "₲"},
	"SRD": {"SRD", "Surinamese Dollar", "$"},
	"UYU": {"UYU", "Uruguayan Peso", "$U"},
	"VES": {"VES", "Venezuelan Bolívar", "Bs."},
	"ANG": {"ANG", "Netherlands Antillean Guilder", "ƒ"},
	"FKP": {"FKP", "Falkland Islands Pound", "£"},
	"BZD": {"BZD", "Belize Dollar", "BZ$"},
	"GIP": {"GIP", "Gibraltar Pound", "£"},
	"SHP": {"SHP", "Saint Helena Pound", "£"},
	"AWG": {"AWG", "Aruban Florin", "ƒ"},
	"PGK": {"PGK", "Papua New Guinean Kina", "K"},
	"TOP": {"TOP", "Tongan Paʻanga", "T$"},
	"SBD": {"SBD", "Solomon Islands Dollar", "SI$"},
	"VUV": {"VUV", "Vanuatuan Vatu", "Vt"},
	"FJD": {"FJD", "Fijian Dollar", "FJ$"},
	"WST": {"WST", "Samoan Tala", "T"},
	"KPW": {"KPW", "North Korean Won", "₩"},
	"CUP": {"CUP", "Cuban Peso", "₱"},
}

// PhoneCountryCodeData 电话区码数据
var PhoneCountryCodeData = []struct {
	Code      string
	Name      string
	IsDefault bool
}{
	{"+852", "Hong Kong", true},
	{"+853", "Macau", false},
	{"+86", "China", false},
	{"+886", "Taiwan", false},
	{"+81", "Japan", false},
	{"+82", "South Korea", false},
	{"+1", "United States/Canada", false},
	{"+44", "United Kingdom", false},
	{"+61", "Australia", false},
	{"+65", "Singapore", false},
	{"+60", "Malaysia", false},
	{"+62", "Indonesia", false},
	{"+63", "Philippines", false},
	{"+66", "Thailand", false},
	{"+84", "Vietnam", false},
	{"+971", "United Arab Emirates", false},
	{"+91", "India", false},
	{"+33", "France", false},
	{"+49", "Germany", false},
	{"+39", "Italy", false},
	{"+34", "Spain", false},
	{"+41", "Switzerland", false},
	{"+7", "Russia", false},
	{"+55", "Brazil", false},
	{"+52", "Mexico", false},
	{"+54", "Argentina", false},
	{"+64", "New Zealand", false},
	{"+27", "South Africa", false},
	{"+20", "Egypt", false},
	{"+30", "Greece", false},
	{"+31", "Netherlands", false},
	{"+32", "Belgium", false},
	{"+36", "Hungary", false},
	{"+40", "Romania", false},
	{"+43", "Austria", false},
	{"+45", "Denmark", false},
	{"+46", "Sweden", false},
	{"+47", "Norway", false},
	{"+48", "Poland", false},
	{"+51", "Peru", false},
	{"+53", "Cuba", false},
	{"+56", "Chile", false},
	{"+57", "Colombia", false},
	{"+58", "Venezuela", false},
	{"+90", "Turkey", false},
	{"+92", "Pakistan", false},
	{"+93", "Afghanistan", false},
	{"+94", "Sri Lanka", false},
	{"+95", "Myanmar", false},
	{"+98", "Iran", false},
	{"+212", "Morocco", false},
	{"+213", "Algeria", false},
	{"+216", "Tunisia", false},
	{"+218", "Libya", false},
	{"+220", "Gambia", false},
	{"+221", "Senegal", false},
	{"+222", "Mauritania", false},
	{"+223", "Mali", false},
	{"+224", "Guinea", false},
	{"+225", "Ivory Coast", false},
	{"+226", "Burkina Faso", false},
	{"+227", "Niger", false},
	{"+228", "Togo", false},
	{"+229", "Benin", false},
	{"+230", "Mauritius", false},
	{"+231", "Liberia", false},
	{"+232", "Sierra Leone", false},
	{"+233", "Ghana", false},
	{"+234", "Nigeria", false},
	{"+235", "Chad", false},
	{"+236", "Central African Republic", false},
	{"+237", "Cameroon", false},
	{"+238", "Cape Verde", false},
	{"+239", "São Tomé and Príncipe", false},
	{"+240", "Equatorial Guinea", false},
	{"+241", "Gabon", false},
	{"+242", "Republic of the Congo", false},
	{"+243", "Democratic Republic of the Congo", false},
	{"+244", "Angola", false},
	{"+245", "Guinea-Bissau", false},
	{"+246", "British Indian Ocean Territory", false},
	{"+248", "Seychelles", false},
	{"+249", "Sudan", false},
	{"+250", "Rwanda", false},
	{"+251", "Ethiopia", false},
	{"+252", "Somalia", false},
	{"+253", "Djibouti", false},
	{"+254", "Kenya", false},
	{"+255", "Tanzania", false},
	{"+256", "Uganda", false},
	{"+257", "Burundi", false},
	{"+258", "Mozambique", false},
	{"+260", "Zambia", false},
	{"+261", "Madagascar", false},
	{"+262", "Réunion / Mayotte", false},
	{"+263", "Zimbabwe", false},
	{"+264", "Namibia", false},
	{"+265", "Malawi", false},
	{"+266", "Lesotho", false},
	{"+267", "Botswana", false},
	{"+268", "Eswatini", false},
	{"+269", "Comoros", false},
	{"+290", "Saint Helena", false},
	{"+291", "Eritrea", false},
	{"+297", "Aruba", false},
	{"+298", "Faroe Islands", false},
	{"+299", "Greenland", false},
	{"+350", "Gibraltar", false},
	{"+351", "Portugal", false},
	{"+352", "Luxembourg", false},
	{"+353", "Ireland", false},
	{"+354", "Iceland", false},
	{"+355", "Albania", false},
	{"+356", "Malta", false},
	{"+357", "Cyprus", false},
	{"+358", "Finland", false},
	{"+359", "Bulgaria", false},
	{"+370", "Lithuania", false},
	{"+371", "Latvia", false},
	{"+372", "Estonia", false},
	{"+373", "Moldova", false},
	{"+374", "Armenia", false},
	{"+375", "Belarus", false},
	{"+376", "Andorra", false},
	{"+377", "Monaco", false},
	{"+378", "San Marino", false},
	{"+380", "Ukraine", false},
	{"+381", "Serbia", false},
	{"+382", "Montenegro", false},
	{"+383", "Kosovo", false},
	{"+385", "Croatia", false},
	{"+386", "Slovenia", false},
	{"+387", "Bosnia and Herzegovina", false},
	{"+389", "North Macedonia", false},
	{"+420", "Czech Republic", false},
	{"+421", "Slovakia", false},
	{"+423", "Liechtenstein", false},
	{"+500", "Falkland Islands", false},
	{"+501", "Belize", false},
	{"+502", "Guatemala", false},
	{"+503", "El Salvador", false},
	{"+504", "Honduras", false},
	{"+505", "Nicaragua", false},
	{"+506", "Costa Rica", false},
	{"+507", "Panama", false},
	{"+508", "Saint Pierre and Miquelon", false},
	{"+509", "Haiti", false},
	{"+590", "Guadeloupe", false},
	{"+591", "Bolivia", false},
	{"+592", "Guyana", false},
	{"+593", "Ecuador", false},
	{"+594", "French Guiana", false},
	{"+595", "Paraguay", false},
	{"+596", "Martinique", false},
	{"+597", "Suriname", false},
	{"+598", "Uruguay", false},
	{"+599", "Netherlands Antilles", false},
	{"+670", "East Timor", false},
	{"+672", "Australian External Territories", false},
	{"+673", "Brunei", false},
	{"+674", "Nauru", false},
	{"+675", "Papua New Guinea", false},
	{"+676", "Tonga", false},
	{"+677", "Solomon Islands", false},
	{"+678", "Vanuatu", false},
	{"+679", "Fiji", false},
	{"+680", "Palau", false},
	{"+681", "Wallis and Futuna", false},
	{"+682", "Cook Islands", false},
	{"+683", "Niue", false},
	{"+685", "Samoa", false},
	{"+686", "Kiribati", false},
	{"+687", "New Caledonia", false},
	{"+688", "Tuvalu", false},
	{"+689", "French Polynesia", false},
	{"+690", "Tokelau", false},
	{"+691", "Micronesia", false},
	{"+692", "Marshall Islands", false},
	{"+850", "North Korea", false},
	{"+855", "Cambodia", false},
	{"+856", "Laos", false},
	{"+880", "Bangladesh", false},
	{"+960", "Maldives", false},
	{"+961", "Lebanon", false},
	{"+962", "Jordan", false},
	{"+963", "Syria", false},
	{"+964", "Iraq", false},
	{"+965", "Kuwait", false},
	{"+966", "Saudi Arabia", false},
	{"+967", "Yemen", false},
	{"+968", "Oman", false},
	{"+970", "Palestine", false},
	{"+972", "Israel", false},
	{"+973", "Bahrain", false},
	{"+974", "Qatar", false},
	{"+975", "Bhutan", false},
	{"+976", "Mongolia", false},
	{"+977", "Nepal", false},
	{"+992", "Tajikistan", false},
	{"+993", "Turkmenistan", false},
	{"+994", "Azerbaijan", false},
	{"+995", "Georgia", false},
	{"+996", "Kyrgyzstan", false},
	{"+998", "Uzbekistan", false},
}

// InitTenantData 初始化租户的预设数据
func InitTenantData(tenantID uuid.UUID, defaultPhoneCode string) error {
	now := time.Now()

	// 1. 初始化电话区码
	log.Printf("初始化租户 %s 的电话区码数据...", tenantID)
	for _, phoneData := range PhoneCountryCodeData {
		phoneCode := models.PhoneCountryCode{
			ID:        uuid.New(),
			TenantID:  tenantID,
			Code:      phoneData.Code,
			Name:      phoneData.Name,
			IsDefault: phoneData.Code == defaultPhoneCode, // 如果与注册时的区码相同，设为默认
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := database.DB.Create(&phoneCode).Error; err != nil {
			log.Printf("⚠️  创建电话区码失败 %s: %v", phoneData.Code, err)
			// 继续处理其他数据，不中断
		}
	}

	// 2. 初始化货币（根据电话区码映射）
	log.Printf("初始化租户 %s 的货币数据...", tenantID)
	currencyMap := make(map[string]bool) // 用于去重
	var defaultCurrencyCode string       // 记录默认货币代码
	
	// 先找到默认电话区码对应的货币
	if defaultPhoneCode != "" {
		if currencyCode, exists := PhoneCodeToCurrency[defaultPhoneCode]; exists {
			defaultCurrencyCode = currencyCode
		}
	}
	
	for _, phoneData := range PhoneCountryCodeData {
		currencyCode, exists := PhoneCodeToCurrency[phoneData.Code]
		if !exists {
			// 如果没有映射，使用 USD 作为默认
			currencyCode = "USD"
		}

		// 避免重复创建相同的货币
		if currencyMap[currencyCode] {
			continue
		}

		currencyInfo, exists := CurrencyData[currencyCode]
		if !exists {
			// 如果货币信息不存在，使用默认值
			currencyInfo = CurrencyInfo{
				Code:   currencyCode,
				Name:   currencyCode,
				Symbol: currencyCode,
			}
		}

		// 如果这是注册时的默认电话区码对应的货币，设为默认货币
		isDefault := currencyCode == defaultCurrencyCode

		currency := models.Currency{
			ID:           uuid.New(),
			TenantID:     tenantID,
			Code:         currencyInfo.Code,
			Name:         currencyInfo.Name,
			Symbol:       &currencyInfo.Symbol,
			ExchangeRate: 1.0,
			IsDefault:    isDefault,
			Status:       "active",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := database.DB.Create(&currency).Error; err != nil {
			log.Printf("⚠️  创建货币失败 %s: %v", currencyInfo.Code, err)
			// 继续处理其他数据，不中断
		} else {
			// 标记已创建，避免重复
			currencyMap[currencyCode] = true
		}
	}

	// 3. 初始化角色（admin role）
	log.Printf("初始化租户 %s 的角色数据...", tenantID)
	adminRole := models.Role{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Name:        "Admin",
		Description: "System administrator with full permissions",
		Permissions: models.StringArrayJSONB{},
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := database.DB.Create(&adminRole).Error; err != nil {
		log.Printf("⚠️  创建角色失败: %v", err)
		return err
	}

	// 4. 初始化默认付款方式（Cash）
	log.Printf("初始化租户 %s 的默认付款方式...", tenantID)
	cashPayment := models.PaymentMethod{
		ID:              uuid.New(),
		TenantID:        tenantID,
		Name:            "Cash",
		Code:            "cash",
		IsDefault:       true,  // 系统预设客户付款方法
		IsDefaultExpense: true, // 系统预设支出付款方法
		IsOnlinePayment: false,
		Status:          "active",
		ExtraFields:     models.JSONB{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := database.DB.Create(&cashPayment).Error; err != nil {
		log.Printf("⚠️  创建默认付款方式失败: %v", err)
		// 继续处理其他数据，不中断
	}

	// 5. 初始化默认运送方式
	log.Printf("初始化租户 %s 的默认运送方式...", tenantID)
	// 店取
	pickupShipping := models.ShippingMethod{
		ID:               uuid.New(),
		TenantID:         tenantID,
		Name:             "店取",
		Code:             "pickup",
		RequiresShipping: false,
		IsDefault:        true, // 默认选择
		Status:           "active",
		ExtraFields:      models.JSONB{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := database.DB.Create(&pickupShipping).Error; err != nil {
		log.Printf("⚠️  创建店取运送方式失败: %v", err)
		// 继续处理其他数据，不中断
	}

	// 送货上门（系统预设）
	deliveryShipping := models.ShippingMethod{
		ID:               uuid.New(),
		TenantID:         tenantID,
		Name:             "送貨上門",
		Code:             "delivery",
		RequiresShipping: true,
		IsDefault:        false,
		Status:           "active",
		ExtraFields:      models.JSONB{"is_system_default": true}, // 标记为系统预设
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := database.DB.Create(&deliveryShipping).Error; err != nil {
		log.Printf("⚠️  创建送货上门运送方式失败: %v", err)
		// 继续处理其他数据，不中断
	}

	// 6. 初始化默认会员等级（4个等级）
	log.Printf("初始化租户 %s 的默认会员等级...", tenantID)
	memberLevels := []struct {
		Name              string
		Code              string
		LevelOrder        int
		MinPoints         int
		MinPurchaseAmount float64
		DiscountRate      float64
		IsDefault         bool
		Description       string
	}{
		{"Regular", "regular", 1, 0, 0.00, 0.00, true, "Regular member level"},
		{"Silver", "silver", 2, 100, 1000.00, 5.00, false, "Silver member level"},
		{"Gold", "gold", 3, 500, 5000.00, 10.00, false, "Gold member level"},
		{"Diamond", "diamond", 4, 1000, 10000.00, 15.00, false, "Diamond member level"},
	}

	for _, levelData := range memberLevels {
		memberLevel := models.MemberLevel{
			ID:                uuid.New(),
			TenantID:          tenantID,
			Name:              levelData.Name,
			Code:              &levelData.Code,
			LevelOrder:        levelData.LevelOrder,
			MinPoints:         levelData.MinPoints,
			MinPurchaseAmount: levelData.MinPurchaseAmount,
			DiscountRate:      levelData.DiscountRate,
			IsDefault:         levelData.IsDefault,
			AutoUpgrade:       false,
			Description:       levelData.Description,
			Benefits:          models.JSONB{},
			Status:            "active",
			ExtraFields:       models.JSONB{},
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := database.DB.Create(&memberLevel).Error; err != nil {
			log.Printf("⚠️  创建会员等级失败 %s: %v", levelData.Name, err)
			// 继续处理其他数据，不中断
		}
	}

	// 7. 初始化默认工作时段（9:00 AM - 6:00 PM）
	log.Printf("初始化租户 %s 的默认工作时段...", tenantID)
	
	// 先检查是否已存在默认工作时段
	var existingDefaultShift models.Shift
	if err := database.DB.Where("tenant_id = ? AND is_default = ?", tenantID, true).First(&existingDefaultShift).Error; err != nil {
		// 不存在，创建新的默认工作时段
		defaultShift := models.Shift{
			ID:        uuid.New(),
			TenantID:  tenantID,
			Name:      "Default",
			StartTime: models.NewSQLTime(time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC)),  // 9:00 AM
			EndTime:   models.NewSQLTime(time.Date(0, 1, 1, 18, 0, 0, 0, time.UTC)), // 6:00 PM
			IsDefault: true,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := database.DB.Create(&defaultShift).Error; err != nil {
			log.Printf("⚠️  创建默认工作时段失败: %v", err)
			// 继续处理其他数据，不中断
		} else {
			// 将租户中所有没有 shift_id 的用户关联到默认工作时段
			database.DB.Model(&models.User{}).
				Where("tenant_id = ? AND shift_id IS NULL", tenantID).
				Update("shift_id", defaultShift.ID)
		}
	} else {
		// 已存在默认工作时段，将租户中所有没有 shift_id 的用户关联到默认工作时段
		database.DB.Model(&models.User{}).
			Where("tenant_id = ? AND shift_id IS NULL", tenantID).
			Update("shift_id", existingDefaultShift.ID)
	}

	// 8. 初始化默认订单标签（订单样板）
	log.Printf("初始化租户 %s 的默认订单标签...", tenantID)
	defaultOrderLabel := models.OrderLabel{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Name:      "訂單樣板",
		Color:     "#007bff",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := database.DB.Create(&defaultOrderLabel).Error; err != nil {
		log.Printf("⚠️  创建默认订单标签失败: %v", err)
		// 继续处理其他数据，不中断
	}

	log.Printf("✅ 租户 %s 的预设数据初始化完成", tenantID)
	return nil
}

