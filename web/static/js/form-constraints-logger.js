(function () {
  function normalizeName(el) {
    return el.getAttribute('name') || el.getAttribute('id') || '(anonymous)';
  }

  function pickAttr(el, name) {
    const v = el.getAttribute(name);
    return v === null || v === '' ? null : v;
  }

  function collectDataRules(el) {
    const data = {};
    Array.from(el.attributes || []).forEach((attr) => {
      if (!attr.name || !attr.name.startsWith('data-')) return;
      const key = attr.name.replace(/^data-/, '');
      if (!key) return;
      if (!/(rule|rules|validate|validation|pattern|min|max|length|required)/i.test(key)) return;
      data[key] = attr.value;
    });
    return data;
  }

  function collectFieldConstraints(el) {
    const constraints = {
      type: el.getAttribute('type') || el.tagName.toLowerCase(),
      required: el.hasAttribute('required'),
      minLength: el.hasAttribute('minlength') ? Number(el.getAttribute('minlength')) : null,
      maxLength: el.hasAttribute('maxlength') ? Number(el.getAttribute('maxlength')) : null,
      pattern: pickAttr(el, 'pattern'),
      min: pickAttr(el, 'min'),
      max: pickAttr(el, 'max'),
      step: pickAttr(el, 'step'),
      inputMode: pickAttr(el, 'inputmode'),
      autoComplete: pickAttr(el, 'autocomplete'),
      placeholder: pickAttr(el, 'placeholder'),
      readOnly: el.hasAttribute('readonly'),
      disabled: el.hasAttribute('disabled')
    };

    const dataRules = collectDataRules(el);
    if (Object.keys(dataRules).length) {
      constraints.dataRules = dataRules;
    }

    return constraints;
  }

  function logAllForms() {
    const forms = Array.from(document.querySelectorAll('form'));
    if (!forms.length) {
      console.info('[FormConstraints] No forms found.');
      return;
    }

    console.group('[FormConstraints] Forms summary');
    forms.forEach((form, formIndex) => {
      const formId = form.getAttribute('id') || form.getAttribute('name') || `form-${formIndex + 1}`;
      console.group(`[FormConstraints] ${formId}`);

      const fields = Array.from(form.querySelectorAll('input, select, textarea'));
      if (!fields.length) {
        console.info('No fields');
        console.groupEnd();
        return;
      }

      fields.forEach((field) => {
        const name = normalizeName(field);
        const constraints = collectFieldConstraints(field);
        console.log(name, constraints);
      });

      console.groupEnd();
    });
    console.groupEnd();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', logAllForms);
  } else {
    logAllForms();
  }
})();
