/**
 * vAI Application Center (vai-apps.js)
 * 130 AI application templates across 13 categories.
 * Clicking an app navigates to /vai-chat and sends the pre-built prompt.
 */
var VaiApps = (function () {
  'use strict';

  // ─── Helpers ─────────────────────────────────────────────
  function t(key, fallback) {
    if (typeof I18n !== 'undefined' && I18n.t) return I18n.t(key) || fallback;
    return fallback;
  }

  // ─── Category Definitions ───────────────────────────────
  var CATEGORIES = [
    { id: 'vwork',        icon: 'bi-box-seam',            colorBg: '#dbeafe', color: '#1d4ed8', i18n: 'vai.apps.cat.vwork' },
    { id: 'writing',      icon: 'bi-pencil-square',      colorBg: '#ede9fe', color: '#8b5cf6', i18n: 'vai.apps.cat.writing' },
    { id: 'business',     icon: 'bi-briefcase',           colorBg: '#fef3c7', color: '#d97706', i18n: 'vai.apps.cat.business' },
    { id: 'development',  icon: 'bi-code-slash',          colorBg: '#e0f2fe', color: '#0284c7', i18n: 'vai.apps.cat.development' },
    { id: 'education',    icon: 'bi-mortarboard',         colorBg: '#fce7f3', color: '#db2777', i18n: 'vai.apps.cat.education' },
    { id: 'marketing',    icon: 'bi-megaphone',           colorBg: '#ede9fe', color: '#7c3aed', i18n: 'vai.apps.cat.marketing' },
    { id: 'lifestyle',    icon: 'bi-heart',               colorBg: '#fef2f2', color: '#ef4444', i18n: 'vai.apps.cat.lifestyle' },
    { id: 'support',      icon: 'bi-headset',             colorBg: '#ecfdf5', color: '#059669', i18n: 'vai.apps.cat.support' },
    { id: 'hr',           icon: 'bi-people',              colorBg: '#fff7ed', color: '#ea580c', i18n: 'vai.apps.cat.hr' },
    { id: 'design',       icon: 'bi-palette',             colorBg: '#fdf4ff', color: '#c026d3', i18n: 'vai.apps.cat.design' },
    { id: 'legal',        icon: 'bi-shield-check',        colorBg: '#f0fdf4', color: '#16a34a', i18n: 'vai.apps.cat.legal' },
    { id: 'analytics',    icon: 'bi-bar-chart-line',      colorBg: '#eff6ff', color: '#2563eb', i18n: 'vai.apps.cat.analytics' },
    { id: 'translation',  icon: 'bi-translate',           colorBg: '#f0fdfa', color: '#0d9488', i18n: 'vai.apps.cat.translation' }
  ];

  // ─── App Definitions (130 apps) ─────────────────────────
  // Each: { id, category, icon, i18nName, i18nDesc, i18nPrompt }
  var APPS = [
    // ── vWork 專屬功能 (vWork) ──
    { id: 'vwOrderMgmt',     category: 'vwork', icon: 'bi-cart-check',          i18nName: 'vai.apps.app.vwOrderMgmt.name',     i18nDesc: 'vai.apps.app.vwOrderMgmt.desc',     i18nPrompt: 'vai.apps.app.vwOrderMgmt.prompt' },
    { id: 'vwInventory',     category: 'vwork', icon: 'bi-box-seam',            i18nName: 'vai.apps.app.vwInventory.name',      i18nDesc: 'vai.apps.app.vwInventory.desc',      i18nPrompt: 'vai.apps.app.vwInventory.prompt' },
    { id: 'vwCustomerMgmt',  category: 'vwork', icon: 'bi-person-rolodex',      i18nName: 'vai.apps.app.vwCustomerMgmt.name',   i18nDesc: 'vai.apps.app.vwCustomerMgmt.desc',   i18nPrompt: 'vai.apps.app.vwCustomerMgmt.prompt' },
    { id: 'vwInvoice',       category: 'vwork', icon: 'bi-receipt',             i18nName: 'vai.apps.app.vwInvoice.name',        i18nDesc: 'vai.apps.app.vwInvoice.desc',        i18nPrompt: 'vai.apps.app.vwInvoice.prompt' },
    { id: 'vwReport',        category: 'vwork', icon: 'bi-file-earmark-bar-graph', i18nName: 'vai.apps.app.vwReport.name',      i18nDesc: 'vai.apps.app.vwReport.desc',      i18nPrompt: 'vai.apps.app.vwReport.prompt' },
    { id: 'vwWorkflow',      category: 'vwork', icon: 'bi-diagram-3',           i18nName: 'vai.apps.app.vwWorkflow.name',       i18nDesc: 'vai.apps.app.vwWorkflow.desc',       i18nPrompt: 'vai.apps.app.vwWorkflow.prompt' },
    { id: 'vwSchedule',      category: 'vwork', icon: 'bi-calendar2-week',      i18nName: 'vai.apps.app.vwSchedule.name',       i18nDesc: 'vai.apps.app.vwSchedule.desc',       i18nPrompt: 'vai.apps.app.vwSchedule.prompt' },
    { id: 'vwApproval',      category: 'vwork', icon: 'bi-check2-circle',       i18nName: 'vai.apps.app.vwApproval.name',       i18nDesc: 'vai.apps.app.vwApproval.desc',       i18nPrompt: 'vai.apps.app.vwApproval.prompt' },
    { id: 'vwNotification',  category: 'vwork', icon: 'bi-bell',                i18nName: 'vai.apps.app.vwNotification.name',   i18nDesc: 'vai.apps.app.vwNotification.desc',   i18nPrompt: 'vai.apps.app.vwNotification.prompt' },
    { id: 'vwDataImport',    category: 'vwork', icon: 'bi-cloud-upload',        i18nName: 'vai.apps.app.vwDataImport.name',     i18nDesc: 'vai.apps.app.vwDataImport.desc',     i18nPrompt: 'vai.apps.app.vwDataImport.prompt' },

    // ── 寫作助手 (Writing) ──
    { id: 'article',          category: 'writing',   icon: 'bi-file-text',           i18nName: 'vai.apps.app.article.name',         i18nDesc: 'vai.apps.app.article.desc',         i18nPrompt: 'vai.apps.app.article.prompt' },
    { id: 'blog',             category: 'writing',   icon: 'bi-journal-text',        i18nName: 'vai.apps.app.blog.name',            i18nDesc: 'vai.apps.app.blog.desc',            i18nPrompt: 'vai.apps.app.blog.prompt' },
    { id: 'adCopy',           category: 'writing',   icon: 'bi-badge-ad',            i18nName: 'vai.apps.app.adCopy.name',          i18nDesc: 'vai.apps.app.adCopy.desc',          i18nPrompt: 'vai.apps.app.adCopy.prompt' },
    { id: 'productDesc',      category: 'writing',   icon: 'bi-bag',                 i18nName: 'vai.apps.app.productDesc.name',     i18nDesc: 'vai.apps.app.productDesc.desc',     i18nPrompt: 'vai.apps.app.productDesc.prompt' },
    { id: 'pressRelease',     category: 'writing',   icon: 'bi-newspaper',           i18nName: 'vai.apps.app.pressRelease.name',    i18nDesc: 'vai.apps.app.pressRelease.desc',    i18nPrompt: 'vai.apps.app.pressRelease.prompt' },
    { id: 'socialPost',       category: 'writing',   icon: 'bi-chat-square-heart',   i18nName: 'vai.apps.app.socialPost.name',      i18nDesc: 'vai.apps.app.socialPost.desc',      i18nPrompt: 'vai.apps.app.socialPost.prompt' },
    { id: 'seoArticle',       category: 'writing',   icon: 'bi-search',              i18nName: 'vai.apps.app.seoArticle.name',      i18nDesc: 'vai.apps.app.seoArticle.desc',      i18nPrompt: 'vai.apps.app.seoArticle.prompt' },
    { id: 'story',            category: 'writing',   icon: 'bi-book',                i18nName: 'vai.apps.app.story.name',           i18nDesc: 'vai.apps.app.story.desc',           i18nPrompt: 'vai.apps.app.story.prompt' },
    { id: 'speech',           category: 'writing',   icon: 'bi-mic',                 i18nName: 'vai.apps.app.speech.name',          i18nDesc: 'vai.apps.app.speech.desc',          i18nPrompt: 'vai.apps.app.speech.prompt' },
    { id: 'newsletter',       category: 'writing',   icon: 'bi-envelope-paper',      i18nName: 'vai.apps.app.newsletter.name',      i18nDesc: 'vai.apps.app.newsletter.desc',      i18nPrompt: 'vai.apps.app.newsletter.prompt' },

    // ── 商業工具 (Business) ──
    { id: 'bizPlan',          category: 'business',  icon: 'bi-clipboard-data',      i18nName: 'vai.apps.app.bizPlan.name',         i18nDesc: 'vai.apps.app.bizPlan.desc',         i18nPrompt: 'vai.apps.app.bizPlan.prompt' },
    { id: 'swot',             category: 'business',  icon: 'bi-grid',                i18nName: 'vai.apps.app.swot.name',            i18nDesc: 'vai.apps.app.swot.desc',            i18nPrompt: 'vai.apps.app.swot.prompt' },
    { id: 'marketAnalysis',   category: 'business',  icon: 'bi-graph-up',            i18nName: 'vai.apps.app.marketAnalysis.name',  i18nDesc: 'vai.apps.app.marketAnalysis.desc',  i18nPrompt: 'vai.apps.app.marketAnalysis.prompt' },
    { id: 'meetingNotes',     category: 'business',  icon: 'bi-journal-check',       i18nName: 'vai.apps.app.meetingNotes.name',    i18nDesc: 'vai.apps.app.meetingNotes.desc',    i18nPrompt: 'vai.apps.app.meetingNotes.prompt' },
    { id: 'proposal',         category: 'business',  icon: 'bi-file-earmark-richtext', i18nName: 'vai.apps.app.proposal.name',      i18nDesc: 'vai.apps.app.proposal.desc',      i18nPrompt: 'vai.apps.app.proposal.prompt' },
    { id: 'financialReport',  category: 'business',  icon: 'bi-cash-stack',          i18nName: 'vai.apps.app.financialReport.name', i18nDesc: 'vai.apps.app.financialReport.desc', i18nPrompt: 'vai.apps.app.financialReport.prompt' },
    { id: 'investPitch',      category: 'business',  icon: 'bi-rocket-takeoff',      i18nName: 'vai.apps.app.investPitch.name',     i18nDesc: 'vai.apps.app.investPitch.desc',     i18nPrompt: 'vai.apps.app.investPitch.prompt' },
    { id: 'competitorAnalysis', category: 'business', icon: 'bi-binoculars',         i18nName: 'vai.apps.app.competitorAnalysis.name', i18nDesc: 'vai.apps.app.competitorAnalysis.desc', i18nPrompt: 'vai.apps.app.competitorAnalysis.prompt' },
    { id: 'okr',              category: 'business',  icon: 'bi-bullseye',            i18nName: 'vai.apps.app.okr.name',             i18nDesc: 'vai.apps.app.okr.desc',             i18nPrompt: 'vai.apps.app.okr.prompt' },
    { id: 'riskAssessment',   category: 'business',  icon: 'bi-exclamation-triangle', i18nName: 'vai.apps.app.riskAssessment.name', i18nDesc: 'vai.apps.app.riskAssessment.desc', i18nPrompt: 'vai.apps.app.riskAssessment.prompt' },

    // ── 程式開發 (Development) ──
    { id: 'codeGen',          category: 'development', icon: 'bi-code-square',       i18nName: 'vai.apps.app.codeGen.name',         i18nDesc: 'vai.apps.app.codeGen.desc',         i18nPrompt: 'vai.apps.app.codeGen.prompt' },
    { id: 'codeReview',       category: 'development', icon: 'bi-eye',               i18nName: 'vai.apps.app.codeReview.name',      i18nDesc: 'vai.apps.app.codeReview.desc',      i18nPrompt: 'vai.apps.app.codeReview.prompt' },
    { id: 'debugHelper',      category: 'development', icon: 'bi-bug',               i18nName: 'vai.apps.app.debugHelper.name',     i18nDesc: 'vai.apps.app.debugHelper.desc',     i18nPrompt: 'vai.apps.app.debugHelper.prompt' },
    { id: 'apiDoc',           category: 'development', icon: 'bi-box-arrow-in-right', i18nName: 'vai.apps.app.apiDoc.name',         i18nDesc: 'vai.apps.app.apiDoc.desc',         i18nPrompt: 'vai.apps.app.apiDoc.prompt' },
    { id: 'sqlQuery',         category: 'development', icon: 'bi-database',          i18nName: 'vai.apps.app.sqlQuery.name',        i18nDesc: 'vai.apps.app.sqlQuery.desc',        i18nPrompt: 'vai.apps.app.sqlQuery.prompt' },
    { id: 'regex',            category: 'development', icon: 'bi-regex',             i18nName: 'vai.apps.app.regex.name',           i18nDesc: 'vai.apps.app.regex.desc',           i18nPrompt: 'vai.apps.app.regex.prompt' },
    { id: 'architecture',     category: 'development', icon: 'bi-diagram-3',         i18nName: 'vai.apps.app.architecture.name',    i18nDesc: 'vai.apps.app.architecture.desc',    i18nPrompt: 'vai.apps.app.architecture.prompt' },
    { id: 'unitTest',         category: 'development', icon: 'bi-check2-square',     i18nName: 'vai.apps.app.unitTest.name',        i18nDesc: 'vai.apps.app.unitTest.desc',        i18nPrompt: 'vai.apps.app.unitTest.prompt' },
    { id: 'refactor',         category: 'development', icon: 'bi-arrow-repeat',      i18nName: 'vai.apps.app.refactor.name',        i18nDesc: 'vai.apps.app.refactor.desc',        i18nPrompt: 'vai.apps.app.refactor.prompt' },
    { id: 'techDoc',          category: 'development', icon: 'bi-file-earmark-code', i18nName: 'vai.apps.app.techDoc.name',         i18nDesc: 'vai.apps.app.techDoc.desc',         i18nPrompt: 'vai.apps.app.techDoc.prompt' },

    // ── 教育學習 (Education) ──
    { id: 'studyPlan',        category: 'education', icon: 'bi-calendar-check',      i18nName: 'vai.apps.app.studyPlan.name',       i18nDesc: 'vai.apps.app.studyPlan.desc',       i18nPrompt: 'vai.apps.app.studyPlan.prompt' },
    { id: 'examPrep',         category: 'education', icon: 'bi-journal-bookmark',    i18nName: 'vai.apps.app.examPrep.name',        i18nDesc: 'vai.apps.app.examPrep.desc',        i18nPrompt: 'vai.apps.app.examPrep.prompt' },
    { id: 'thesisOutline',    category: 'education', icon: 'bi-list-nested',         i18nName: 'vai.apps.app.thesisOutline.name',   i18nDesc: 'vai.apps.app.thesisOutline.desc',   i18nPrompt: 'vai.apps.app.thesisOutline.prompt' },
    { id: 'langLearn',        category: 'education', icon: 'bi-globe2',              i18nName: 'vai.apps.app.langLearn.name',       i18nDesc: 'vai.apps.app.langLearn.desc',       i18nPrompt: 'vai.apps.app.langLearn.prompt' },
    { id: 'explainer',        category: 'education', icon: 'bi-lightbulb',           i18nName: 'vai.apps.app.explainer.name',       i18nDesc: 'vai.apps.app.explainer.desc',       i18nPrompt: 'vai.apps.app.explainer.prompt' },
    { id: 'readingNotes',     category: 'education', icon: 'bi-bookmark-star',       i18nName: 'vai.apps.app.readingNotes.name',    i18nDesc: 'vai.apps.app.readingNotes.desc',    i18nPrompt: 'vai.apps.app.readingNotes.prompt' },
    { id: 'courseDesign',      category: 'education', icon: 'bi-easel',               i18nName: 'vai.apps.app.courseDesign.name',     i18nDesc: 'vai.apps.app.courseDesign.desc',     i18nPrompt: 'vai.apps.app.courseDesign.prompt' },
    { id: 'quizGen',          category: 'education', icon: 'bi-question-circle',     i18nName: 'vai.apps.app.quizGen.name',         i18nDesc: 'vai.apps.app.quizGen.desc',         i18nPrompt: 'vai.apps.app.quizGen.prompt' },
    { id: 'research',         category: 'education', icon: 'bi-search-heart',        i18nName: 'vai.apps.app.research.name',        i18nDesc: 'vai.apps.app.research.desc',        i18nPrompt: 'vai.apps.app.research.prompt' },
    { id: 'flashcard',        category: 'education', icon: 'bi-card-text',           i18nName: 'vai.apps.app.flashcard.name',       i18nDesc: 'vai.apps.app.flashcard.desc',       i18nPrompt: 'vai.apps.app.flashcard.prompt' },

    // ── 行銷推廣 (Marketing) ──
    { id: 'marketingStrategy', category: 'marketing', icon: 'bi-graph-up-arrow',    i18nName: 'vai.apps.app.marketingStrategy.name', i18nDesc: 'vai.apps.app.marketingStrategy.desc', i18nPrompt: 'vai.apps.app.marketingStrategy.prompt' },
    { id: 'brandNaming',       category: 'marketing', icon: 'bi-tag',               i18nName: 'vai.apps.app.brandNaming.name',     i18nDesc: 'vai.apps.app.brandNaming.desc',     i18nPrompt: 'vai.apps.app.brandNaming.prompt' },
    { id: 'slogan',            category: 'marketing', icon: 'bi-chat-quote',        i18nName: 'vai.apps.app.slogan.name',          i18nDesc: 'vai.apps.app.slogan.desc',          i18nPrompt: 'vai.apps.app.slogan.prompt' },
    { id: 'emailMarketing',    category: 'marketing', icon: 'bi-envelope-at',       i18nName: 'vai.apps.app.emailMarketing.name',  i18nDesc: 'vai.apps.app.emailMarketing.desc',  i18nPrompt: 'vai.apps.app.emailMarketing.prompt' },
    { id: 'socialStrategy',    category: 'marketing', icon: 'bi-share',             i18nName: 'vai.apps.app.socialStrategy.name',  i18nDesc: 'vai.apps.app.socialStrategy.desc',  i18nPrompt: 'vai.apps.app.socialStrategy.prompt' },
    { id: 'kolCollab',         category: 'marketing', icon: 'bi-person-video3',     i18nName: 'vai.apps.app.kolCollab.name',       i18nDesc: 'vai.apps.app.kolCollab.desc',       i18nPrompt: 'vai.apps.app.kolCollab.prompt' },
    { id: 'promoPlanning',     category: 'marketing', icon: 'bi-gift',              i18nName: 'vai.apps.app.promoPlanning.name',   i18nDesc: 'vai.apps.app.promoPlanning.desc',   i18nPrompt: 'vai.apps.app.promoPlanning.prompt' },
    { id: 'persona',           category: 'marketing', icon: 'bi-person-badge',      i18nName: 'vai.apps.app.persona.name',         i18nDesc: 'vai.apps.app.persona.desc',         i18nPrompt: 'vai.apps.app.persona.prompt' },
    { id: 'abTest',            category: 'marketing', icon: 'bi-toggles',           i18nName: 'vai.apps.app.abTest.name',          i18nDesc: 'vai.apps.app.abTest.desc',          i18nPrompt: 'vai.apps.app.abTest.prompt' },
    { id: 'adPlacement',       category: 'marketing', icon: 'bi-cursor-fill',       i18nName: 'vai.apps.app.adPlacement.name',     i18nDesc: 'vai.apps.app.adPlacement.desc',     i18nPrompt: 'vai.apps.app.adPlacement.prompt' },

    // ── 日常生活 (Lifestyle) ──
    { id: 'travelPlan',       category: 'lifestyle', icon: 'bi-airplane',           i18nName: 'vai.apps.app.travelPlan.name',      i18nDesc: 'vai.apps.app.travelPlan.desc',      i18nPrompt: 'vai.apps.app.travelPlan.prompt' },
    { id: 'recipe',            category: 'lifestyle', icon: 'bi-egg-fried',          i18nName: 'vai.apps.app.recipe.name',          i18nDesc: 'vai.apps.app.recipe.desc',          i18nPrompt: 'vai.apps.app.recipe.prompt' },
    { id: 'fitnessPlan',       category: 'lifestyle', icon: 'bi-heart-pulse',        i18nName: 'vai.apps.app.fitnessPlan.name',     i18nDesc: 'vai.apps.app.fitnessPlan.desc',     i18nPrompt: 'vai.apps.app.fitnessPlan.prompt' },
    { id: 'financePlanning',   category: 'lifestyle', icon: 'bi-piggy-bank',        i18nName: 'vai.apps.app.financePlanning.name', i18nDesc: 'vai.apps.app.financePlanning.desc', i18nPrompt: 'vai.apps.app.financePlanning.prompt' },
    { id: 'giftIdea',          category: 'lifestyle', icon: 'bi-gift',              i18nName: 'vai.apps.app.giftIdea.name',        i18nDesc: 'vai.apps.app.giftIdea.desc',        i18nPrompt: 'vai.apps.app.giftIdea.prompt' },
    { id: 'partyPlanning',     category: 'lifestyle', icon: 'bi-balloon',           i18nName: 'vai.apps.app.partyPlanning.name',   i18nDesc: 'vai.apps.app.partyPlanning.desc',   i18nPrompt: 'vai.apps.app.partyPlanning.prompt' },
    { id: 'homeDecor',         category: 'lifestyle', icon: 'bi-house-heart',       i18nName: 'vai.apps.app.homeDecor.name',       i18nDesc: 'vai.apps.app.homeDecor.desc',       i18nPrompt: 'vai.apps.app.homeDecor.prompt' },
    { id: 'petCare',           category: 'lifestyle', icon: 'bi-emoji-heart-eyes',  i18nName: 'vai.apps.app.petCare.name',         i18nDesc: 'vai.apps.app.petCare.desc',         i18nPrompt: 'vai.apps.app.petCare.prompt' },
    { id: 'timeManagement',    category: 'lifestyle', icon: 'bi-clock',             i18nName: 'vai.apps.app.timeManagement.name',  i18nDesc: 'vai.apps.app.timeManagement.desc',  i18nPrompt: 'vai.apps.app.timeManagement.prompt' },
    { id: 'habitBuilding',     category: 'lifestyle', icon: 'bi-check2-all',        i18nName: 'vai.apps.app.habitBuilding.name',   i18nDesc: 'vai.apps.app.habitBuilding.desc',   i18nPrompt: 'vai.apps.app.habitBuilding.prompt' },

    // ── 客服支援 (Support) ──
    { id: 'faqGen',            category: 'support',   icon: 'bi-patch-question',     i18nName: 'vai.apps.app.faqGen.name',          i18nDesc: 'vai.apps.app.faqGen.desc',          i18nPrompt: 'vai.apps.app.faqGen.prompt' },
    { id: 'complaintReply',    category: 'support',   icon: 'bi-reply',              i18nName: 'vai.apps.app.complaintReply.name',  i18nDesc: 'vai.apps.app.complaintReply.desc',  i18nPrompt: 'vai.apps.app.complaintReply.prompt' },
    { id: 'refundProcess',     category: 'support',   icon: 'bi-arrow-return-left',  i18nName: 'vai.apps.app.refundProcess.name',   i18nDesc: 'vai.apps.app.refundProcess.desc',   i18nPrompt: 'vai.apps.app.refundProcess.prompt' },
    { id: 'productGuide',      category: 'support',   icon: 'bi-info-circle',        i18nName: 'vai.apps.app.productGuide.name',    i18nDesc: 'vai.apps.app.productGuide.desc',    i18nPrompt: 'vai.apps.app.productGuide.prompt' },
    { id: 'serviceFlow',       category: 'support',   icon: 'bi-diagram-2',          i18nName: 'vai.apps.app.serviceFlow.name',     i18nDesc: 'vai.apps.app.serviceFlow.desc',     i18nPrompt: 'vai.apps.app.serviceFlow.prompt' },
    { id: 'userGuide',         category: 'support',   icon: 'bi-book-half',          i18nName: 'vai.apps.app.userGuide.name',       i18nDesc: 'vai.apps.app.userGuide.desc',       i18nPrompt: 'vai.apps.app.userGuide.prompt' },
    { id: 'complaintAnalysis', category: 'support',   icon: 'bi-clipboard2-data',   i18nName: 'vai.apps.app.complaintAnalysis.name', i18nDesc: 'vai.apps.app.complaintAnalysis.desc', i18nPrompt: 'vai.apps.app.complaintAnalysis.prompt' },
    { id: 'satisfactionSurvey', category: 'support',  icon: 'bi-star-half',          i18nName: 'vai.apps.app.satisfactionSurvey.name', i18nDesc: 'vai.apps.app.satisfactionSurvey.desc', i18nPrompt: 'vai.apps.app.satisfactionSurvey.prompt' },
    { id: 'scriptTemplate',   category: 'support',   icon: 'bi-chat-left-text',     i18nName: 'vai.apps.app.scriptTemplate.name',  i18nDesc: 'vai.apps.app.scriptTemplate.desc',  i18nPrompt: 'vai.apps.app.scriptTemplate.prompt' },
    { id: 'knowledgeBase',     category: 'support',   icon: 'bi-database-gear',      i18nName: 'vai.apps.app.knowledgeBase.name',   i18nDesc: 'vai.apps.app.knowledgeBase.desc',   i18nPrompt: 'vai.apps.app.knowledgeBase.prompt' },

    // ── 人力資源 (HR) ──
    { id: 'jobDescription',   category: 'hr',        icon: 'bi-file-person',         i18nName: 'vai.apps.app.jobDescription.name',  i18nDesc: 'vai.apps.app.jobDescription.desc',  i18nPrompt: 'vai.apps.app.jobDescription.prompt' },
    { id: 'interviewQ',       category: 'hr',        icon: 'bi-question-diamond',    i18nName: 'vai.apps.app.interviewQ.name',      i18nDesc: 'vai.apps.app.interviewQ.desc',      i18nPrompt: 'vai.apps.app.interviewQ.prompt' },
    { id: 'offerLetter',      category: 'hr',        icon: 'bi-envelope-check',      i18nName: 'vai.apps.app.offerLetter.name',     i18nDesc: 'vai.apps.app.offerLetter.desc',     i18nPrompt: 'vai.apps.app.offerLetter.prompt' },
    { id: 'performanceReview', category: 'hr',       icon: 'bi-speedometer2',        i18nName: 'vai.apps.app.performanceReview.name', i18nDesc: 'vai.apps.app.performanceReview.desc', i18nPrompt: 'vai.apps.app.performanceReview.prompt' },
    { id: 'employeeHandbook', category: 'hr',        icon: 'bi-book',                i18nName: 'vai.apps.app.employeeHandbook.name', i18nDesc: 'vai.apps.app.employeeHandbook.desc', i18nPrompt: 'vai.apps.app.employeeHandbook.prompt' },
    { id: 'trainingPlan',     category: 'hr',        icon: 'bi-person-workspace',    i18nName: 'vai.apps.app.trainingPlan.name',    i18nDesc: 'vai.apps.app.trainingPlan.desc',    i18nPrompt: 'vai.apps.app.trainingPlan.prompt' },
    { id: 'exitInterview',    category: 'hr',        icon: 'bi-door-open',           i18nName: 'vai.apps.app.exitInterview.name',   i18nDesc: 'vai.apps.app.exitInterview.desc',   i18nPrompt: 'vai.apps.app.exitInterview.prompt' },
    { id: 'salaryReport',     category: 'hr',        icon: 'bi-currency-dollar',     i18nName: 'vai.apps.app.salaryReport.name',    i18nDesc: 'vai.apps.app.salaryReport.desc',    i18nPrompt: 'vai.apps.app.salaryReport.prompt' },
    { id: 'teamBuilding',     category: 'hr',        icon: 'bi-people-fill',         i18nName: 'vai.apps.app.teamBuilding.name',    i18nDesc: 'vai.apps.app.teamBuilding.desc',    i18nPrompt: 'vai.apps.app.teamBuilding.prompt' },
    { id: 'companyCulture',   category: 'hr',        icon: 'bi-building-heart',      i18nName: 'vai.apps.app.companyCulture.name',  i18nDesc: 'vai.apps.app.companyCulture.desc',  i18nPrompt: 'vai.apps.app.companyCulture.prompt' },

    // ── 創意設計 (Design) ──
    { id: 'logoConcept',      category: 'design',    icon: 'bi-vector-pen',          i18nName: 'vai.apps.app.logoConcept.name',     i18nDesc: 'vai.apps.app.logoConcept.desc',     i18nPrompt: 'vai.apps.app.logoConcept.prompt' },
    { id: 'colorScheme',      category: 'design',    icon: 'bi-palette2',            i18nName: 'vai.apps.app.colorScheme.name',     i18nDesc: 'vai.apps.app.colorScheme.desc',     i18nPrompt: 'vai.apps.app.colorScheme.prompt' },
    { id: 'uiCopy',           category: 'design',    icon: 'bi-window',              i18nName: 'vai.apps.app.uiCopy.name',          i18nDesc: 'vai.apps.app.uiCopy.desc',          i18nPrompt: 'vai.apps.app.uiCopy.prompt' },
    { id: 'brandStory',       category: 'design',    icon: 'bi-heart',               i18nName: 'vai.apps.app.brandStory.name',      i18nDesc: 'vai.apps.app.brandStory.desc',      i18nPrompt: 'vai.apps.app.brandStory.prompt' },
    { id: 'namingIdea',       category: 'design',    icon: 'bi-lightbulb',           i18nName: 'vai.apps.app.namingIdea.name',      i18nDesc: 'vai.apps.app.namingIdea.desc',      i18nPrompt: 'vai.apps.app.namingIdea.prompt' },
    { id: 'taglineDesign',    category: 'design',    icon: 'bi-quote',               i18nName: 'vai.apps.app.taglineDesign.name',   i18nDesc: 'vai.apps.app.taglineDesign.desc',   i18nPrompt: 'vai.apps.app.taglineDesign.prompt' },
    { id: 'packagingCopy',    category: 'design',    icon: 'bi-box2',                i18nName: 'vai.apps.app.packagingCopy.name',   i18nDesc: 'vai.apps.app.packagingCopy.desc',   i18nPrompt: 'vai.apps.app.packagingCopy.prompt' },
    { id: 'spaceDesc',        category: 'design',    icon: 'bi-building',            i18nName: 'vai.apps.app.spaceDesc.name',       i18nDesc: 'vai.apps.app.spaceDesc.desc',       i18nPrompt: 'vai.apps.app.spaceDesc.prompt' },
    { id: 'styleGuide',       category: 'design',    icon: 'bi-brush',               i18nName: 'vai.apps.app.styleGuide.name',      i18nDesc: 'vai.apps.app.styleGuide.desc',      i18nPrompt: 'vai.apps.app.styleGuide.prompt' },
    { id: 'creativeBrief',    category: 'design',    icon: 'bi-stars',               i18nName: 'vai.apps.app.creativeBrief.name',   i18nDesc: 'vai.apps.app.creativeBrief.desc',   i18nPrompt: 'vai.apps.app.creativeBrief.prompt' },

    // ── 法律合規 (Legal) ──
    { id: 'contractDraft',    category: 'legal',     icon: 'bi-file-earmark-ruled',  i18nName: 'vai.apps.app.contractDraft.name',   i18nDesc: 'vai.apps.app.contractDraft.desc',   i18nPrompt: 'vai.apps.app.contractDraft.prompt' },
    { id: 'privacyPolicy',    category: 'legal',     icon: 'bi-shield-lock',         i18nName: 'vai.apps.app.privacyPolicy.name',   i18nDesc: 'vai.apps.app.privacyPolicy.desc',   i18nPrompt: 'vai.apps.app.privacyPolicy.prompt' },
    { id: 'termsOfUse',       category: 'legal',     icon: 'bi-file-earmark-check',  i18nName: 'vai.apps.app.termsOfUse.name',      i18nDesc: 'vai.apps.app.termsOfUse.desc',      i18nPrompt: 'vai.apps.app.termsOfUse.prompt' },
    { id: 'disclaimer',       category: 'legal',     icon: 'bi-exclamation-circle',  i18nName: 'vai.apps.app.disclaimer.name',      i18nDesc: 'vai.apps.app.disclaimer.desc',      i18nPrompt: 'vai.apps.app.disclaimer.prompt' },
    { id: 'nda',              category: 'legal',     icon: 'bi-lock',                i18nName: 'vai.apps.app.nda.name',             i18nDesc: 'vai.apps.app.nda.desc',             i18nPrompt: 'vai.apps.app.nda.prompt' },
    { id: 'trademark',        category: 'legal',     icon: 'bi-award',               i18nName: 'vai.apps.app.trademark.name',       i18nDesc: 'vai.apps.app.trademark.desc',       i18nPrompt: 'vai.apps.app.trademark.prompt' },
    { id: 'complianceCheck',  category: 'legal',     icon: 'bi-clipboard-check',     i18nName: 'vai.apps.app.complianceCheck.name', i18nDesc: 'vai.apps.app.complianceCheck.desc', i18nPrompt: 'vai.apps.app.complianceCheck.prompt' },
    { id: 'legalOpinion',     category: 'legal',     icon: 'bi-chat-right-text',     i18nName: 'vai.apps.app.legalOpinion.name',    i18nDesc: 'vai.apps.app.legalOpinion.desc',    i18nPrompt: 'vai.apps.app.legalOpinion.prompt' },
    { id: 'riskDisclosure',   category: 'legal',     icon: 'bi-exclamation-diamond', i18nName: 'vai.apps.app.riskDisclosure.name',  i18nDesc: 'vai.apps.app.riskDisclosure.desc',  i18nPrompt: 'vai.apps.app.riskDisclosure.prompt' },
    { id: 'authorization',    category: 'legal',     icon: 'bi-person-check',        i18nName: 'vai.apps.app.authorization.name',   i18nDesc: 'vai.apps.app.authorization.desc',   i18nPrompt: 'vai.apps.app.authorization.prompt' },

    // ── 數據分析 (Analytics) ──
    { id: 'dataInterpret',    category: 'analytics', icon: 'bi-graph-up',            i18nName: 'vai.apps.app.dataInterpret.name',   i18nDesc: 'vai.apps.app.dataInterpret.desc',   i18nPrompt: 'vai.apps.app.dataInterpret.prompt' },
    { id: 'reportDesign',     category: 'analytics', icon: 'bi-layout-text-sidebar-reverse', i18nName: 'vai.apps.app.reportDesign.name', i18nDesc: 'vai.apps.app.reportDesign.desc', i18nPrompt: 'vai.apps.app.reportDesign.prompt' },
    { id: 'kpiSetting',       category: 'analytics', icon: 'bi-speedometer',         i18nName: 'vai.apps.app.kpiSetting.name',      i18nDesc: 'vai.apps.app.kpiSetting.desc',      i18nPrompt: 'vai.apps.app.kpiSetting.prompt' },
    { id: 'trendAnalysis',    category: 'analytics', icon: 'bi-trending-up',         i18nName: 'vai.apps.app.trendAnalysis.name',   i18nDesc: 'vai.apps.app.trendAnalysis.desc',   i18nPrompt: 'vai.apps.app.trendAnalysis.prompt' },
    { id: 'userAnalysis',     category: 'analytics', icon: 'bi-person-lines-fill',   i18nName: 'vai.apps.app.userAnalysis.name',    i18nDesc: 'vai.apps.app.userAnalysis.desc',    i18nPrompt: 'vai.apps.app.userAnalysis.prompt' },
    { id: 'salesAnalysis',    category: 'analytics', icon: 'bi-cart-check',          i18nName: 'vai.apps.app.salesAnalysis.name',   i18nDesc: 'vai.apps.app.salesAnalysis.desc',   i18nPrompt: 'vai.apps.app.salesAnalysis.prompt' },
    { id: 'forecastModel',    category: 'analytics', icon: 'bi-cpu',                 i18nName: 'vai.apps.app.forecastModel.name',   i18nDesc: 'vai.apps.app.forecastModel.desc',   i18nPrompt: 'vai.apps.app.forecastModel.prompt' },
    { id: 'dataCleaning',     category: 'analytics', icon: 'bi-funnel',              i18nName: 'vai.apps.app.dataCleaning.name',    i18nDesc: 'vai.apps.app.dataCleaning.desc',    i18nPrompt: 'vai.apps.app.dataCleaning.prompt' },
    { id: 'vizSuggestion',    category: 'analytics', icon: 'bi-pie-chart',           i18nName: 'vai.apps.app.vizSuggestion.name',   i18nDesc: 'vai.apps.app.vizSuggestion.desc',   i18nPrompt: 'vai.apps.app.vizSuggestion.prompt' },
    { id: 'abExperiment',     category: 'analytics', icon: 'bi-toggles2',            i18nName: 'vai.apps.app.abExperiment.name',    i18nDesc: 'vai.apps.app.abExperiment.desc',    i18nPrompt: 'vai.apps.app.abExperiment.prompt' },

    // ── 翻譯語言 (Translation) ──
    { id: 'zhEnTranslate',    category: 'translation', icon: 'bi-translate',         i18nName: 'vai.apps.app.zhEnTranslate.name',   i18nDesc: 'vai.apps.app.zhEnTranslate.desc',   i18nPrompt: 'vai.apps.app.zhEnTranslate.prompt' },
    { id: 'jaTranslate',      category: 'translation', icon: 'bi-translate',         i18nName: 'vai.apps.app.jaTranslate.name',     i18nDesc: 'vai.apps.app.jaTranslate.desc',     i18nPrompt: 'vai.apps.app.jaTranslate.prompt' },
    { id: 'koTranslate',      category: 'translation', icon: 'bi-translate',         i18nName: 'vai.apps.app.koTranslate.name',     i18nDesc: 'vai.apps.app.koTranslate.desc',     i18nPrompt: 'vai.apps.app.koTranslate.prompt' },
    { id: 'multiTranslate',   category: 'translation', icon: 'bi-globe',             i18nName: 'vai.apps.app.multiTranslate.name',  i18nDesc: 'vai.apps.app.multiTranslate.desc',  i18nPrompt: 'vai.apps.app.multiTranslate.prompt' },
    { id: 'localization',     category: 'translation', icon: 'bi-geo-alt',           i18nName: 'vai.apps.app.localization.name',    i18nDesc: 'vai.apps.app.localization.desc',    i18nPrompt: 'vai.apps.app.localization.prompt' },
    { id: 'subtitleTranslate', category: 'translation', icon: 'bi-badge-cc',         i18nName: 'vai.apps.app.subtitleTranslate.name', i18nDesc: 'vai.apps.app.subtitleTranslate.desc', i18nPrompt: 'vai.apps.app.subtitleTranslate.prompt' },
    { id: 'bizTranslate',     category: 'translation', icon: 'bi-briefcase',         i18nName: 'vai.apps.app.bizTranslate.name',    i18nDesc: 'vai.apps.app.bizTranslate.desc',    i18nPrompt: 'vai.apps.app.bizTranslate.prompt' },
    { id: 'techTranslate',    category: 'translation', icon: 'bi-cpu',               i18nName: 'vai.apps.app.techTranslate.name',   i18nDesc: 'vai.apps.app.techTranslate.desc',   i18nPrompt: 'vai.apps.app.techTranslate.prompt' },
    { id: 'literaryTranslate', category: 'translation', icon: 'bi-book-half',        i18nName: 'vai.apps.app.literaryTranslate.name', i18nDesc: 'vai.apps.app.literaryTranslate.desc', i18nPrompt: 'vai.apps.app.literaryTranslate.prompt' },
    { id: 'colloquialTranslate', category: 'translation', icon: 'bi-chat-left-dots', i18nName: 'vai.apps.app.colloquialTranslate.name', i18nDesc: 'vai.apps.app.colloquialTranslate.desc', i18nPrompt: 'vai.apps.app.colloquialTranslate.prompt' }
  ];

  // ─── State ──────────────────────────────────────────────
  var currentCategory = 'all';
  var searchQuery = '';

  // ─── Render Category Buttons ────────────────────────────
  function renderCategories() {
    var container = document.querySelector('#vaiAppsCategories .vai-apps-categories-inner');
    if (!container) return;
    // Keep the "All" button, append category buttons
    var allBtn = container.querySelector('[data-category="all"]');
    container.innerHTML = '';
    if (allBtn) container.appendChild(allBtn);

    CATEGORIES.forEach(function (cat) {
      var btn = document.createElement('button');
      btn.className = 'vai-apps-category-btn';
      btn.setAttribute('data-category', cat.id);
      btn.onclick = function () { VaiApps.filterCategory(cat.id); };
      btn.innerHTML = '<i class="bi ' + cat.icon + '"></i> <span data-i18n="' + cat.i18n + '">' + t(cat.i18n, cat.id) + '</span>';
      container.appendChild(btn);
    });
  }

  // ─── Render App Grid ────────────────────────────────────
  function renderApps() {
    var grid = document.getElementById('vaiAppsGrid');
    var empty = document.getElementById('vaiAppsEmpty');
    if (!grid) return;

    var filtered = APPS;

    // Category filter
    if (currentCategory !== 'all') {
      filtered = filtered.filter(function (a) { return a.category === currentCategory; });
    }

    // Search filter
    if (searchQuery) {
      var q = searchQuery.toLowerCase();
      filtered = filtered.filter(function (a) {
        var name = t(a.i18nName, '').toLowerCase();
        var desc = t(a.i18nDesc, '').toLowerCase();
        return name.indexOf(q) !== -1 || desc.indexOf(q) !== -1;
      });
    }

    if (!filtered.length) {
      grid.style.display = 'none';
      if (empty) empty.style.display = '';
      return;
    }

    grid.style.display = '';
    if (empty) empty.style.display = 'none';

    // Group by category
    var grouped = {};
    var catOrder = [];
    filtered.forEach(function (app) {
      if (!grouped[app.category]) {
        grouped[app.category] = [];
        catOrder.push(app.category);
      }
      grouped[app.category].push(app);
    });

    var html = '';
    catOrder.forEach(function (catId) {
      var cat = CATEGORIES.find(function (c) { return c.id === catId; });
      if (!cat) return;
      var apps = grouped[catId];
      // Section title (only when showing "all" or searching across categories)
      if (currentCategory === 'all' || searchQuery) {
        html += '<div class="vai-apps-section-title">'
          + '<i class="bi ' + cat.icon + '"></i>'
          + '<span data-i18n="' + cat.i18n + '">' + t(cat.i18n, catId) + '</span>'
          + '<span class="vai-apps-section-count">' + apps.length + '</span>'
          + '</div>';
      }
      apps.forEach(function (app) {
        html += renderAppCard(app, cat);
      });
    });

    grid.innerHTML = html;

    // Apply i18n if available
    if (typeof I18n !== 'undefined' && I18n.applyTranslations) {
      I18n.applyTranslations(grid);
    }
  }

  function renderAppCard(app, cat) {
    return '<div class="vai-app-card" data-app-id="' + app.id + '" onclick="VaiApps.launch(\'' + app.id + '\')">'
      + '<div class="vai-app-card-icon" style="background: ' + cat.colorBg + '; color: ' + cat.color + ';">'
      + '<i class="bi ' + app.icon + '"></i>'
      + '</div>'
      + '<div class="vai-app-card-body">'
      + '<div class="vai-app-card-name" data-i18n="' + app.i18nName + '">' + t(app.i18nName, app.id) + '</div>'
      + '<div class="vai-app-card-desc" data-i18n="' + app.i18nDesc + '">' + t(app.i18nDesc, '') + '</div>'
      + '</div>'
      + '</div>';
  }

  // ─── Category Filter ────────────────────────────────────
  function filterCategory(catId) {
    currentCategory = catId;
    // Update active state
    document.querySelectorAll('.vai-apps-category-btn').forEach(function (btn) {
      btn.classList.toggle('active', btn.getAttribute('data-category') === catId);
    });
    renderApps();
  }

  // ─── Search ─────────────────────────────────────────────
  function search(query) {
    searchQuery = (query || '').trim();
    renderApps();
  }

  // ─── Launch App → Navigate to Chat ──────────────────────
  function launch(appId) {
    var app = APPS.find(function (a) { return a.id === appId; });
    if (!app) return;

    var prompt = t(app.i18nPrompt, '');
    if (!prompt) return;

    // Store prompt in sessionStorage, then navigate to chat
    try {
      sessionStorage.setItem('vai_app_prompt', prompt);
    } catch (e) {}

    window.location.href = '/vai-chat';
  }

  // ─── Init ───────────────────────────────────────────────
  function init() {
    renderCategories();
    renderApps();

    // Re-render when language changes (I18n fires 'languageChanged' event)
    document.addEventListener('languageChanged', function () {
      renderCategories();
      renderApps();
    });
  }

  // ─── Public API ─────────────────────────────────────────
  return {
    init: init,
    filterCategory: filterCategory,
    search: search,
    launch: launch
  };
})();
