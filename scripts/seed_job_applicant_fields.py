# Generate 欄位設定 payload for job_applicants (local vs overseas helper biodata).
import json

YES_NO = [
    {"value": "yes", "label": "Yes 是"},
    {"value": "no", "label": "No 否"},
]
LANG = [
    {"value": "average", "label": "Average 平"},
    {"value": "good", "label": "Good 好"},
    {"value": "excellent", "label": "Excellent 優"},
]
SKILL = [
    {"value": "none", "label": "—"},
    {"value": "willing", "label": "願意 Willing"},
    {"value": "experienced", "label": "經驗 Experienced"},
    {"value": "both", "label": "願意+經驗"},
]
SEX = [
    {"value": "F", "label": "Female 女"},
    {"value": "M", "label": "Male 男"},
]
MARITAL = [
    {"value": "single", "label": "Single 未婚"},
    {"value": "married", "label": "Married 已婚"},
    {"value": "divorced", "label": "Divorced 離婚"},
    {"value": "widowed", "label": "Widowed 喪偶"},
]


def extra(key, label, typ="text", dep=None, **kw):
    f = {"key": key, "label": label, "type": typ, "required": False, "isExtra": True}
    if dep:
        f["dependency"] = {"field": "worker_type", "value": dep}
    if typ in ("textarea", "section"):
        f["fullWidth"] = True
    f.update(kw)
    return f


def emp_local(prefix, n, dep="local"):
    title = f"僱傭經驗 {n}"
    return [
        extra(f"{prefix}_sec", f"DOMESTIC EMPLOYMENT RECORDS 僱傭經驗 {n}", "section", dep),
        extra(f"{prefix}_employer_location", "Location of Employer 僱主地區", "text", dep),
        extra(f"{prefix}_house_size", "How big the house? 房屋面積", "text", dep),
        extra(f"{prefix}_from", "From 由", "date", dep),
        extra(f"{prefix}_to", "To 至", "date", dep),
        extra(f"{prefix}_employer_name", "Employer's Name 僱主姓名", "text", dep),
        extra(f"{prefix}_reason_leave", "Reason to leave 離職原因", "text", dep),
        extra(f"{prefix}_newborn", "How many care of New Born Babies? 照顧初生嬰兒人數", "number", dep),
        extra(f"{prefix}_pets", "How many care of Pets? 照顧寵物數量", "number", dep),
        extra(f"{prefix}_children", "How many care of Children? 照顧小孩人數", "number", dep),
        extra(f"{prefix}_children_ages", "Their Ages? 小孩年齡", "text", dep),
        extra(f"{prefix}_adults", "No. of Adults 成人數", "number", dep),
        extra(f"{prefix}_elderly", "How many care of Elderly Persons? 照顧長者人數", "number", dep),
        extra(f"{prefix}_elderly_ages", "Their Ages? 長者年齡", "text", dep),
        extra(f"{prefix}_salary", "Salary 薪金", "text", dep),
        extra(f"{prefix}_duties", "Duties 工作範圍", "textarea", dep),
    ]


def emp_nondom(prefix, n, dep="local"):
    return [
        extra(f"{prefix}_sec", f"Non-DOMESTIC EMPLOYMENT RECORDS 其他工作經驗 {n}", "section", dep),
        extra(f"{prefix}_employer_location", "Location of Employer 僱主地區", "text", dep),
        extra(f"{prefix}_from", "From 由", "date", dep),
        extra(f"{prefix}_to", "To 至", "date", dep),
        extra(f"{prefix}_employer_name", "Employer's Name 僱主姓名", "text", dep),
        extra(f"{prefix}_reason_leave", "Reason to leave 離職原因", "text", dep),
        extra(f"{prefix}_position", "Position 職位", "text", dep),
        extra(f"{prefix}_salary", "Salary 薪金", "text", dep),
        extra(f"{prefix}_job_desc", "Description of Job 工作描述", "textarea", dep),
    ]


def emp_overseas(prefix, n, dep="overseas"):
    return [
        extra(f"{prefix}_sec", f"Domestic Employment Records 工作紀錄 {n}", "section", dep),
        extra(f"{prefix}_employer_name", "Name of Employer 僱主姓名", "text", dep),
        extra(f"{prefix}_employer_location", "Location of Employer 僱主地區", "text", dep),
        extra(f"{prefix}_working_area", "Working Area 工作面積 (M2)", "text", dep),
        extra(f"{prefix}_phone", "Telephone No. 電話", "text", dep),
        extra(f"{prefix}_from", "From 由", "date", dep),
        extra(f"{prefix}_to", "To 至", "date", dep),
        extra(f"{prefix}_duty_newborn", "Caring of New born baby 照顧初生嬰兒", "select", dep, options=YES_NO),
        extra(f"{prefix}_duty_children", "Caring of Children 照顧小孩", "select", dep, options=YES_NO),
        extra(f"{prefix}_duty_elderly", "Caring of Elderly 照顧長者", "select", dep, options=YES_NO),
        extra(f"{prefix}_duty_disabled", "Caring of Disabled 照顧傷殘人士", "select", dep, options=YES_NO),
        extra(f"{prefix}_reason_quit", "Reason to Quit 離職原因", "text", dep),
        extra(f"{prefix}_family_ages", "Age of family members on first day 到任時家庭成員年齡", "textarea", dep),
    ]


local_qs = [
    "Care of babies aged 0-3 months 照顧 0-3 個月嬰兒",
    "Care of babies aged 3-12 months 照顧 3-12 個月嬰兒",
    "Care of children aged 1-5 yrs old 照顧 1-5 歲小孩",
    "Care of children aged 5-10 yrs old 照顧 5-10 歲小孩",
    "Tutoring children 教導小孩功課",
    "Play with children 陪小孩玩",
    "Operate washing machine 使用洗衣機",
    "Laundry by hands 手洗衣服",
    "Ironing 燙衫",
    "Cook Chinese food 煮中菜",
    "Gardening 園藝",
    "Washing car 洗車",
    "Care of pets 照顧寵物",
    "Look after elderly person 照顧長者",
    "Look after invalid person 照顧傷殘人士",
    "Look after semi-invalid person 照顧半傷殘人士",
]

overseas_skills = [
    "Taking care of New Born to 1 year old Babies 照顧初生至一歲大嬰兒",
    "Take care of 1-5 years old Children 照顧一歲至五歲小孩子",
    "Take care of 5-10 years old Children 照顧五歲至十歲小孩子",
    "Take care of 10-18 years old Children 照顧十歲至十八歲小孩子",
    "Tutoring English Homework for ages 10 or below 英語教導 10 歲以下功課",
    "Looking after Male Disable Person 照顧行動不便的男性",
    "Looking after Female Disable Person 照顧行動不便的女性",
    "Looking after over 60 year old Male Elderly Person 照顧 60 歲以上男性長者",
    "Looking after over 60 year old Female Elderly Person 照顧 60 歲以上女性長者",
    "Looking after Male Patient 照顧男性病人",
    "Looking after Female Patient 照顧女性病人",
    "Care of Small Dog 照顧小狗",
    "Care of Big Dog 照顧大狗",
    "Care of Pets 照顧寵物",
    "General House Works 一般家務",
    "Operating Electrical Appliances 使用家用電器",
    "Doing Laundry by hands 手洗衣服",
    "Ironing 燙衫",
    "Car Washing 抺車",
    "Gardening 料理花園，園藝",
    "Cooking Chinese Foods 煮中國菜",
    "Cooking Western Foods 煮西餐",
    "Eat Pork 吃豬肉",
    "Eat Beef 吃牛肉",
    "Prepare food for 10 persons or above 處理 10 人以上膳食",
    "Prepare party food for 20 persons or above 處理 20 人以上派對膳食",
    "Week Day Holiday 平日休假",
]

extras = []
extras += emp_local("loc_emp1", 1)
extras += emp_local("loc_emp2", 2)
extras += emp_nondom("loc_nd1", 1)
extras += [
    extra("loc_exp_sec", "經驗描述 Experience", "section", "local"),
    extra("loc_exp_baby", "Tell us something about your experience in baby / children care 嬰兒／小孩照顧經驗", "textarea", "local"),
    extra("loc_exp_cooking", "Tell us about your cooking experience 煮食經驗", "textarea", "local"),
    extra("loc_exp_elderly", "Tell us about your experience in taking care of elderly, invalid / semi-invalid person 照顧長者／傷殘人士經驗", "textarea", "local"),
    extra("loc_q_sec", "ANSWER SHEET 問卷", "section", "local"),
]
for i, label in enumerate(local_qs, 1):
    extras.append(extra(f"loc_q{i:02d}", f"{i}. {label}", "select", "local", options=YES_NO))
extras += [
    extra("loc_health_sec", "Health & Legal 健康及法律", "section", "local"),
    extra("loc_smoke", "Do you smoke? 你是否吸煙？", "select", "local", options=YES_NO),
    extra("loc_alcohol", "Do you drink alcohol? 你是否喝酒？", "select", "local", options=YES_NO),
    extra("loc_physical_defects", "Are you have any physical defects? 你是否有任何身體缺陷？", "textarea", "local"),
    extra("loc_allergy", "Do you suffer from any allergy? 你是否有過敏？", "textarea", "local"),
    extra("loc_convicted", "Have you been convicted of any crimes or offences either in Hong Kong or elsewhere? 你是否曾在香港或其他地方被定罪？", "textarea", "local"),
    extra("loc_remarks", "REMARKS 備註", "textarea", "local"),
]

extras += [
    extra("ov_personal_sec", "Personal Data 個人紀錄", "section", "overseas"),
    extra("ov_ref_no", "Ref No. 檔案編號", "text", "overseas"),
    extra("ov_nationality", "Nationality 國籍", "text", "overseas"),
    extra("ov_dob", "Date of Birth 出生日期", "date", "overseas"),
    extra("ov_place_of_birth", "Place of Birth 出生地點", "text", "overseas"),
    extra("ov_age", "Age 年齡", "number", "overseas"),
    extra("ov_religion", "Religion 宗教", "text", "overseas"),
    extra("ov_sex", "Sex 性別", "select", "overseas", options=SEX),
    extra("ov_marital", "Marital Status 婚姻狀況", "select", "overseas", options=MARITAL),
    extra("ov_height", "Height 身高", "text", "overseas"),
    extra("ov_weight", "Weight 體重", "text", "overseas"),
    extra("ov_horoscope_cn", "Chinese Horoscope 生肖", "text", "overseas"),
    extra("ov_horoscope", "Horoscope 星座", "text", "overseas"),
    extra("ov_overseas_address", "Overseas Address 原居地址", "textarea", "overseas"),
    extra("ov_lang_sec", "Languages 語言", "section", "overseas"),
    extra("ov_cantonese", "Spoken Cantonese 能講廣東話", "select", "overseas", options=LANG),
    extra("ov_mandarin", "Spoken Mandarin 能講國語", "select", "overseas", options=LANG),
    extra("ov_english_spoken", "Spoken English 能講英語", "select", "overseas", options=LANG),
    extra("ov_english_written", "Written English 能寫英語", "select", "overseas", options=LANG),
    extra("ov_country_sec", "Other Country Experience 其他國家經驗（年）", "section", "overseas"),
    extra("ov_exp_hk", "Hong Kong 香港 (Year)", "text", "overseas"),
    extra("ov_exp_sg", "Singapore 新加坡 (Year)", "text", "overseas"),
    extra("ov_exp_my", "Malaysia 馬來西亞 (Year)", "text", "overseas"),
    extra("ov_exp_tw", "Taiwan 台灣 (Year)", "text", "overseas"),
    extra("ov_exp_sa", "Saudi Arabia 沙地阿拉伯 (Year)", "text", "overseas"),
    extra("ov_exp_kr", "South Korea 南韓 (Year)", "text", "overseas"),
    extra("ov_exp_other", "Other country 其他國家", "text", "overseas"),
    extra("ov_memo", "Memo 備忘錄", "textarea", "overseas"),
    extra("ov_family_sec", "Family Background 家庭背景", "section", "overseas"),
    extra("ov_father_name", "Father's Name 父親姓名", "text", "overseas"),
    extra("ov_father_occupation", "Father's Occupation 父親職業", "text", "overseas"),
    extra("ov_mother_name", "Mother's Name 母親姓名", "text", "overseas"),
    extra("ov_mother_occupation", "Mother's Occupation 母親職業", "text", "overseas"),
    extra("ov_spouse_name", "Spouse's Name 配偶姓名", "text", "overseas"),
    extra("ov_spouse_occupation", "Spouse's Occupation 配偶職業", "text", "overseas"),
    extra("ov_brothers_ages", "Age of Brothers 兄弟年齡", "text", "overseas"),
    extra("ov_sisters_ages", "Age of Sisters 姊妹年齡", "text", "overseas"),
    extra("ov_sons_ages", "Age of Sons 兒子年齡", "text", "overseas"),
    extra("ov_daughters_ages", "Age of Daughters 女兒年齡", "text", "overseas"),
    extra("ov_family_rank", "In the family, I am no. 在家排行", "text", "overseas"),
    extra("ov_edu_sec", "Educational Details 學歷", "section", "overseas"),
    extra("ov_edu_elementary", "Elementary 小學 (From-To)", "text", "overseas"),
    extra("ov_edu_junior", "Junior High School 初中 (From-To)", "text", "overseas"),
    extra("ov_edu_senior", "Senior High School 高中 (From-To)", "text", "overseas"),
    extra("ov_edu_college", "College 學院 (From-To)", "text", "overseas"),
    extra("ov_edu_university", "University 大學 (From-To)", "text", "overseas"),
    extra("ov_skill_sec", "Special Skill / License 特殊技能及駕照", "section", "overseas"),
    extra("ov_special_skill", "Special Skill 特殊技能", "textarea", "overseas"),
    extra("ov_intl_license_years", "International Drive License Years 國際駕駛執照年資", "text", "overseas"),
    extra("ov_intl_license_since", "International Drive License Since 國際駕駛執照自", "text", "overseas"),
    extra("ov_hk_license_years", "Hong Kong Drive License Years 香港駕駛執照年資", "text", "overseas"),
    extra("ov_hk_license_since", "Hong Kong Drive License Since 香港駕駛執照自", "text", "overseas"),
    extra("ov_other_training", "Other Training 其他訓練", "textarea", "overseas"),
    extra("ov_exp_work_sec", "Experience as Domestic Helper 家傭經驗（願意 / 經驗）", "section", "overseas"),
]
for i, label in enumerate(overseas_skills, 1):
    extras.append(extra(f"ov_skill_{i:02d}", f"{i}. {label}", "select", "overseas", options=SKILL))
extras += emp_overseas("ov_emp1", 1)
extras += emp_overseas("ov_emp2", 2)
extras += emp_overseas("ov_emp3", 3)

base_fields = [
    "vacancy_id",
    "worker_type",
    "candidate_name",
    "candidate_last_name",
    "email",
    "phone",
    "profile_pic",
    "status",
]
fields = [{"key": k, "visible": True, "order": i} for i, k in enumerate(base_fields)]
for i, ef in enumerate(extras):
    fields.append({"key": ef["key"], "visible": True, "order": len(base_fields) + i})
fields.append({"key": "notes", "visible": True, "order": len(base_fields) + len(extras)})

payload = {
    "field_config": {
        "fields": fields,
        "extraFields": extras,
    }
}

out = r"C:\Users\tednv\OneDrive\Desktop\autowork\vwork\scripts\job_applicant_field_settings.json"
with open(out, "w", encoding="utf-8") as f:
    json.dump(payload, f, ensure_ascii=False, indent=2)
print("wrote", out)
print("extraFields", len(extras), "fields", len(fields))
