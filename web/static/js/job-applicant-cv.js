(function (global) {
    const ov = (key, label, type, extra) => Object.assign({
        key, label, type, required: false, isExtra: true,
        dependency: { field: 'worker_type', value: 'overseas' }
    }, extra || {});

    function paperBiodataExtraFields() {
        const jobs = [];
        [1, 2, 3].forEach((n) => {
            const p = `ov_emp${n}`;
            jobs.push(
                ov(`${p}_house_size`, `How big the house? 房屋面積 (${n})`, 'text'),
                ov(`${p}_newborn`, `How many care of New Born Babies? 初生嬰兒 (${n})`, 'number'),
                ov(`${p}_children`, `How many care of Children? 小孩人數 (${n})`, 'number'),
                ov(`${p}_children_ages`, `Their Ages? 小孩年齡 (${n})`, 'text'),
                ov(`${p}_pets`, `How many care of Pets? 寵物 (${n})`, 'number'),
                ov(`${p}_adults`, `No. of Adults 成人數 (${n})`, 'number'),
                ov(`${p}_elderly`, `How many care of Elderly Persons? 長者 (${n})`, 'number'),
                ov(`${p}_salary`, `Salary 薪金 (${n})`, 'text'),
                ov(`${p}_duties`, `Duties 工作範圍 (${n})`, 'textarea', { fullWidth: true })
            );
        });
        return [
            ov('ov_passport', 'Passport No. 護照號碼', 'text'),
            ov('ov_why_hk', 'Why do you want to work in Hong Kong? 來港工作原因', 'textarea', { fullWidth: true }),
            ov('ov_father_age', "Father's Age 父親年齡", 'text'),
            ov('ov_mother_age', "Mother's Age 母親年齡", 'text'),
            ov('ov_spouse_age', "Spouse's Age 配偶年齡", 'text'),
            ov('ov_brothers', 'No. of Brothers 兄弟人數', 'text'),
            ov('ov_sisters', 'No. of Sisters 姊妹人數', 'text'),
            ov('ov_children_count', 'No. of Children 子女人數', 'text'),
            ...jobs
        ];
    }

    function mergePaperBiodataFields(settings) {
        if (!settings) return settings;
        if (!Array.isArray(settings.extraFields)) settings.extraFields = [];
        if (!Array.isArray(settings.fields)) settings.fields = [];
        const have = new Set(settings.extraFields.map((f) => f && f.key).filter(Boolean));
        const fieldKeys = new Set(settings.fields.map((f) => f && f.key).filter(Boolean));
        let order = settings.fields.reduce((m, f) => Math.max(m, Number(f.order) || 0), 0);
        paperBiodataExtraFields().forEach((f) => {
            if (!have.has(f.key)) {
                settings.extraFields.push(f);
                have.add(f.key);
            }
            if (!fieldKeys.has(f.key)) {
                order += 1;
                settings.fields.push({ key: f.key, visible: true, order });
                fieldKeys.add(f.key);
            }
        });
        return settings;
    }

    function esc(v) {
        if (v == null || v === '') return '';
        return String(v)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    function xf(extra, key) {
        if (!extra || extra[key] == null || extra[key] === '') return '';
        return extra[key];
    }

    function fullName(item) {
        return [item.candidate_name, item.candidate_last_name].filter(Boolean).join(' ').trim();
    }

    function optLabel(v) {
        const map = {
            F: 'Female 女', M: 'Male 男',
            single: 'Single 未婚', married: 'Married 已婚', divorced: 'Divorced 離婚', widowed: 'Widowed 喪偶',
            average: 'Average 平', good: 'Good 好', excellent: 'Excellent 優',
            yes: 'Yes 是', no: 'No 否'
        };
        return map[v] || v || '';
    }

    function jobBlock(extra, n, local) {
        const p = local ? `loc_emp${n}` : `ov_emp${n}`;
        const loc = xf(extra, `${p}_employer_location`);
        const house = xf(extra, `${p}_house_size`) || xf(extra, `${p}_working_area`);
        const from = xf(extra, `${p}_from`);
        const to = xf(extra, `${p}_to`);
        const emp = xf(extra, `${p}_employer_name`);
        const reason = xf(extra, `${p}_reason_leave`) || xf(extra, `${p}_reason_quit`);
        const newborn = xf(extra, `${p}_newborn`) || xf(extra, `${p}_duty_newborn`);
        const children = xf(extra, `${p}_children`);
        const ages = xf(extra, `${p}_children_ages`) || xf(extra, `${p}_family_ages`);
        const pets = xf(extra, `${p}_pets`);
        const elderly = xf(extra, `${p}_elderly`) || xf(extra, `${p}_duty_elderly`);
        const adults = xf(extra, `${p}_adults`);
        const salary = xf(extra, `${p}_salary`);
        const duties = xf(extra, `${p}_duties`);
        const filled = [loc, house, from, to, emp, reason, children, ages, salary, duties].some(Boolean);
        if (!filled) return '';
        return `
        <div class="cv-job">
          <div class="cv-row">
            <div class="cv-cell"><span class="cv-k">Location of Employer 僱主地區</span><span class="cv-v">${esc(loc)}</span></div>
            <div class="cv-cell"><span class="cv-k">How big the house? 房屋面積</span><span class="cv-v">${esc(house)}</span></div>
            <div class="cv-cell"><span class="cv-k">From 由</span><span class="cv-v">${esc(from)}</span></div>
            <div class="cv-cell"><span class="cv-k">To 至</span><span class="cv-v">${esc(to)}</span></div>
          </div>
          <div class="cv-row">
            <div class="cv-cell"><span class="cv-k">Employer's Name 僱主姓名</span><span class="cv-v">${esc(emp)}</span></div>
            <div class="cv-cell"><span class="cv-k">Reason to leave 離職原因</span><span class="cv-v">${esc(reason)}</span></div>
            <div class="cv-cell"><span class="cv-k">Salary 薪金</span><span class="cv-v">${esc(salary)}</span></div>
            <div class="cv-cell"><span class="cv-k">New born 初生</span><span class="cv-v">${esc(optLabel(newborn))}</span></div>
          </div>
          <div class="cv-row">
            <div class="cv-cell"><span class="cv-k">Children 小孩</span><span class="cv-v">${esc(children)}</span></div>
            <div class="cv-cell"><span class="cv-k">Their Ages? 年齡</span><span class="cv-v">${esc(ages)}</span></div>
            <div class="cv-cell"><span class="cv-k">Pets 寵物</span><span class="cv-v">${esc(pets)}</span></div>
            <div class="cv-cell"><span class="cv-k">Elderly 長者</span><span class="cv-v">${esc(optLabel(elderly))}</span></div>
          </div>
          <div class="cv-row">
            <div class="cv-cell"><span class="cv-k">No. of Adults 成人數</span><span class="cv-v">${esc(adults)}</span></div>
            <div class="cv-cell cv-cell-wide"><span class="cv-k">Duties 工作範圍</span><span class="cv-v">${esc(duties)}</span></div>
          </div>
        </div>`;
    }

    function buildCvHtml(item, company) {
        const extra = item.extra_fields || {};
        const local = item.worker_type === 'local';
        const name = fullName(item);
        const photo = item.profile_pic ? `<img src="${esc(item.profile_pic)}" alt="">` : '';
        const coName = (company && (company.name || company.company_name)) || '';
        const coZh = (company && company.name_zh) || '';
        const coLine = [company && company.address, company && company.phone, company && company.email]
            .filter(Boolean).join('　');
        const jobs = local
            ? (jobBlock(extra, 1, true) + jobBlock(extra, 2, true))
            : (jobBlock(extra, 1, false) + jobBlock(extra, 2, false) + jobBlock(extra, 3, false));

        const familyRows = [
            ['Father 父親', xf(extra, 'ov_father_name'), xf(extra, 'ov_father_age'), xf(extra, 'ov_father_occupation')],
            ['Mother 母親', xf(extra, 'ov_mother_name'), xf(extra, 'ov_mother_age'), xf(extra, 'ov_mother_occupation')],
            ['Brothers 兄弟', xf(extra, 'ov_brothers'), xf(extra, 'ov_brothers_ages'), ''],
            ['Sisters 姊妹', xf(extra, 'ov_sisters'), xf(extra, 'ov_sisters_ages'), ''],
            ["Husband / Spouse 配偶", xf(extra, 'ov_spouse_name'), xf(extra, 'ov_spouse_age'), xf(extra, 'ov_spouse_occupation')],
            ['Children 子女', xf(extra, 'ov_children_count'), [xf(extra, 'ov_sons_ages'), xf(extra, 'ov_daughters_ages')].filter(Boolean).join(' / '), '']
        ].map((r) => `<tr><td>${esc(r[0])}</td><td>${esc(r[1])}</td><td>${esc(r[2])}</td><td>${esc(r[3])}</td></tr>`).join('');

        return `<!DOCTYPE html><html><head><meta charset="utf-8"><title>${esc(name)} CV</title>
<style>
@page { size: A4; margin: 10mm; }
body { font-family: Arial, "Microsoft JhengHei", sans-serif; font-size: 11px; color: #111; margin: 0; }
.cv { max-width: 190mm; margin: 0 auto; }
.cv-head { text-align: center; border-bottom: 2px solid #111; padding-bottom: 6px; margin-bottom: 8px; }
.cv-head h1 { font-size: 18px; margin: 0; }
.cv-head .zh { font-size: 14px; }
.cv-head .meta { font-size: 10px; margin-top: 4px; }
h2 { font-size: 12px; background: #111; color: #fff; padding: 3px 8px; margin: 10px 0 6px; }
.cv-top { display: grid; grid-template-columns: 1fr 28mm; gap: 8px; }
.cv-photo { width: 28mm; height: 34mm; border: 1px solid #333; display: flex; align-items: center; justify-content: center; overflow: hidden; background: #fafafa; }
.cv-photo img { width: 100%; height: 100%; object-fit: cover; }
.grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 2px 10px; }
.line { display: flex; gap: 6px; border-bottom: 1px dotted #999; min-height: 18px; align-items: flex-end; padding: 1px 0; }
.cv-k { color: #444; font-size: 9px; white-space: nowrap; }
.cv-v { flex: 1; font-weight: 600; }
table { width: 100%; border-collapse: collapse; }
th, td { border: 1px solid #333; padding: 3px 5px; text-align: left; }
th { background: #eee; font-size: 10px; }
.cv-job { border: 1px solid #333; margin-bottom: 6px; }
.cv-row { display: grid; grid-template-columns: 1fr 1fr 1fr 1fr; }
.cv-cell { border-right: 1px solid #ddd; border-bottom: 1px solid #ddd; padding: 4px; min-height: 28px; }
.cv-cell:last-child { border-right: 0; }
.cv-cell-wide { grid-column: span 3; }
.why { border: 1px solid #333; min-height: 36px; padding: 6px; }
.no-print { margin: 12px 0; text-align: center; }
@media print { .no-print { display: none; } body { print-color-adjust: exact; -webkit-print-color-adjust: exact; } }
</style></head><body>
<div class="no-print"><button onclick="window.print()">列印 / 另存 PDF</button></div>
<div class="cv">
  <div class="cv-head">
    <h1>${esc(coName)}</h1>
            <div class="zh">${coZh ? esc(coZh) : ''}</div>
    <div class="meta">${esc(coLine)}</div>
  </div>
  <div class="cv-top">
    <div>
      <h2>PERSONAL PARTICULARS 個人資料</h2>
      <div class="grid2">
        <div class="line"><span class="cv-k">Name 姓名</span><span class="cv-v">${esc(name)}</span></div>
        <div class="line"><span class="cv-k">Age 年齡</span><span class="cv-v">${esc(xf(extra, 'ov_age'))}</span></div>
        <div class="line"><span class="cv-k">Address 地址</span><span class="cv-v">${esc(xf(extra, 'ov_overseas_address'))}</span></div>
        <div class="line"><span class="cv-k">Nationality 國籍</span><span class="cv-v">${esc(xf(extra, 'ov_nationality'))}</span></div>
        <div class="line"><span class="cv-k">Date of Birth 出生日期</span><span class="cv-v">${esc(xf(extra, 'ov_dob'))}</span></div>
        <div class="line"><span class="cv-k">Religion 宗教</span><span class="cv-v">${esc(xf(extra, 'ov_religion'))}</span></div>
        <div class="line"><span class="cv-k">Place of Birth 出生地點</span><span class="cv-v">${esc(xf(extra, 'ov_place_of_birth'))}</span></div>
        <div class="line"><span class="cv-k">Weight 體重</span><span class="cv-v">${esc(xf(extra, 'ov_weight'))}</span></div>
        <div class="line"><span class="cv-k">Height 身高</span><span class="cv-v">${esc(xf(extra, 'ov_height'))}</span></div>
        <div class="line"><span class="cv-k">Marital Status 婚姻</span><span class="cv-v">${esc(optLabel(xf(extra, 'ov_marital')))}</span></div>
        <div class="line"><span class="cv-k">Passport No. 護照</span><span class="cv-v">${esc(xf(extra, 'ov_passport'))}</span></div>
        <div class="line"><span class="cv-k">Phone 電話</span><span class="cv-v">${esc(item.phone)}</span></div>
        <div class="line"><span class="cv-k">Cantonese 廣東話</span><span class="cv-v">${esc(optLabel(xf(extra, 'ov_cantonese')))}</span></div>
      </div>
    </div>
    <div class="cv-photo">${photo || '<span>PHOTO</span>'}</div>
  </div>
  ${local ? `
  <h2>REMARKS 備註</h2>
  <div class="why">${esc(xf(extra, 'loc_remarks') || item.notes)}</div>
  ` : `
  <h2>FAMILY BACKGROUND 家庭背景</h2>
  <table><thead><tr><th></th><th>Name 姓名</th><th>Age 年齡</th><th>Occupation 職業</th></tr></thead><tbody>${familyRows}</tbody></table>
  <h2>EDUCATIONAL ATTAINMENT 教育程度</h2>
  <table><thead><tr><th>Level</th><th>Name of School 校名 / From-To</th></tr></thead><tbody>
    <tr><td>Elementary 小學</td><td>${esc(xf(extra, 'ov_edu_elementary'))}</td></tr>
    <tr><td>High School 中學</td><td>${esc(xf(extra, 'ov_edu_senior') || xf(extra, 'ov_edu_junior'))}</td></tr>
    <tr><td>College / Other 其他</td><td>${esc(xf(extra, 'ov_edu_college') || xf(extra, 'ov_edu_university') || xf(extra, 'ov_other_training'))}</td></tr>
  </tbody></table>
  <h2>Why do you want to work in Hong Kong? 來港工作原因</h2>
  <div class="why">${esc(xf(extra, 'ov_why_hk') || item.notes)}</div>
  `}
  <h2>DOMESTIC EMPLOYMENT RECORDS 僱傭經驗</h2>
  ${jobs || '<div class="why">—</div>'}
</div>
<script>window.addEventListener('load', function(){ setTimeout(function(){ window.print(); }, 250); });</script>
</body></html>`;
    }

    async function companyInfo() {
        const out = { name: '', name_zh: '', address: '', phone: '', email: '' };
        try {
            const raw = localStorage.getItem('user');
            const user = raw ? JSON.parse(raw) : {};
            out.name = (user.tenant && user.tenant.name) || user.company_name || document.title.replace(/ -.*/, '') || '';
        } catch (e) { /* ignore */ }
        try {
            if (typeof App !== 'undefined' && App.apiRequest) {
                const stores = await App.apiRequest('/stores?limit=1');
                const list = (stores && (stores.data || stores.items)) || (Array.isArray(stores) ? stores : []);
                const s = list[0];
                if (s) {
                    out.address = s.address || '';
                    out.phone = s.phone || '';
                    out.email = s.email || '';
                    if (s.name && !out.name) out.name = s.name;
                }
            }
        } catch (e) { /* ignore */ }
        return out;
    }

    function openHtml(html) {
        const w = window.open('', '_blank');
        if (!w) {
            if (typeof App !== 'undefined' && App.showAlert) App.showAlert('請允許彈出視窗以匯出 PDF', 'warning');
            return;
        }
        w.document.open();
        w.document.write(html);
        w.document.close();
    }

    async function printFromItem(item) {
        item = item || {};
        if (item.data && !item.candidate_name && !item.extra_fields) item = item.data;
        if (typeof item.extra_fields === 'string') {
            try { item.extra_fields = JSON.parse(item.extra_fields); } catch (e) { item.extra_fields = {}; }
        }
        const company = await companyInfo();
        openHtml(buildCvHtml(item, company));
    }

    async function printFromId(id) {
        if (!id || typeof App === 'undefined') return;
        const item = await App.apiRequest(`/job-applicants/${id}`);
        await printFromItem(item);
    }

    function printFromForm(form) {
        if (!form || typeof form.collectFormData !== 'function') return;
        const data = form.collectFormData() || {};
        if (form.itemId) data.id = form.itemId;
        printFromItem(data);
    }

    global.JobApplicantCV = {
        mergePaperBiodataFields,
        printFromId,
        printFromForm,
        printFromItem
    };
})(window);
