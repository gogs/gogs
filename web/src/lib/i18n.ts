import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import { webContext } from "@/lib/context";
import bgBG from "@/locales/bg-BG.json";
import csCZ from "@/locales/cs-CZ.json";
import deDE from "@/locales/de-DE.json";
import enGB from "@/locales/en-GB.json";
import enUS from "@/locales/en-US.json";
import esES from "@/locales/es-ES.json";
import faIR from "@/locales/fa-IR.json";
import fiFI from "@/locales/fi-FI.json";
import frFR from "@/locales/fr-FR.json";
import glES from "@/locales/gl-ES.json";
import huHU from "@/locales/hu-HU.json";
import idID from "@/locales/id-ID.json";
import itIT from "@/locales/it-IT.json";
import jaJP from "@/locales/ja-JP.json";
import koKR from "@/locales/ko-KR.json";
import lvLV from "@/locales/lv-LV.json";
import mnMN from "@/locales/mn-MN.json";
import nlNL from "@/locales/nl-NL.json";
import plPL from "@/locales/pl-PL.json";
import ptBR from "@/locales/pt-BR.json";
import ptPT from "@/locales/pt-PT.json";
import roRO from "@/locales/ro-RO.json";
import ruRU from "@/locales/ru-RU.json";
import skSK from "@/locales/sk-SK.json";
import srSP from "@/locales/sr-SP.json";
import svSE from "@/locales/sv-SE.json";
import trTR from "@/locales/tr-TR.json";
import ukUA from "@/locales/uk-UA.json";
import viVN from "@/locales/vi-VN.json";
import zhCN from "@/locales/zh-CN.json";
import zhHK from "@/locales/zh-HK.json";
import zhTW from "@/locales/zh-TW.json";

// eslint-disable-next-line import/no-named-as-default-member
void i18n.use(initReactI18next).init({
  resources: {
    "bg-BG": { translation: bgBG },
    "cs-CZ": { translation: csCZ },
    "de-DE": { translation: deDE },
    "en-GB": { translation: enGB },
    "en-US": { translation: enUS },
    "es-ES": { translation: esES },
    "fa-IR": { translation: faIR },
    "fi-FI": { translation: fiFI },
    "fr-FR": { translation: frFR },
    "gl-ES": { translation: glES },
    "hu-HU": { translation: huHU },
    "id-ID": { translation: idID },
    "it-IT": { translation: itIT },
    "ja-JP": { translation: jaJP },
    "ko-KR": { translation: koKR },
    "lv-LV": { translation: lvLV },
    "mn-MN": { translation: mnMN },
    "nl-NL": { translation: nlNL },
    "pl-PL": { translation: plPL },
    "pt-BR": { translation: ptBR },
    "pt-PT": { translation: ptPT },
    "ro-RO": { translation: roRO },
    "ru-RU": { translation: ruRU },
    "sk-SK": { translation: skSK },
    "sr-SP": { translation: srSP },
    "sv-SE": { translation: svSE },
    "tr-TR": { translation: trTR },
    "uk-UA": { translation: ukUA },
    "vi-VN": { translation: viVN },
    "zh-CN": { translation: zhCN },
    "zh-HK": { translation: zhHK },
    "zh-TW": { translation: zhTW },
  },
  lng: webContext.lang,
  fallbackLng: "en-US",
  interpolation: { escapeValue: false, prefix: "{", suffix: "}" },
  returnNull: false,
});

export default i18n;
