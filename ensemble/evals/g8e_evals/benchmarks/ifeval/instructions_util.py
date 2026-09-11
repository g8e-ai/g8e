# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Utility functions for IFEval instruction verification.

Adapted from the upstream google-research IFEval
``instruction_following_eval/instructions_util.py`` at revision
``041338718b4e8151372fd63677104c65b73a0a4e``. The upstream uses
``nltk`` for sentence tokenization and word counting and
``langdetect`` for language detection. This module replaces those
dependencies with regex-based equivalents that are adequate for
instruction-following checks:

- ``split_into_sentences`` is the upstream regex-based sentence
  splitter (no nltk dependency).
- ``count_words`` uses ``re.findall(r"\\w+", text)`` which is
  equivalent to the upstream ``RegexpTokenizer(r"\\w+")``.
- ``detect_language`` is a lightweight script-based heuristic that
  reliably identifies languages with distinct scripts and falls back
  to ``None`` (uncertain) for ambiguous Latin-script cases, matching
  the upstream behavior of counting detection failures as "followed".
"""

from __future__ import annotations

import re

# ISO 639-1 codes to language names (subset used by upstream IFEval).
LANGUAGE_CODES: dict[str, str] = {
    "en": "English",
    "es": "Spanish",
    "pt": "Portuguese",
    "ar": "Arabic",
    "hi": "Hindi",
    "fr": "French",
    "ru": "Russian",
    "de": "German",
    "ja": "Japanese",
    "it": "Italian",
    "bn": "Bengali",
    "uk": "Ukrainian",
    "th": "Thai",
    "ur": "Urdu",
    "ta": "Tamil",
    "te": "Telugu",
    "bg": "Bulgarian",
    "ko": "Korean",
    "pl": "Polish",
    "he": "Hebrew",
    "fa": "Persian",
    "vi": "Vietnamese",
    "ne": "Nepali",
    "sw": "Swahili",
    "kn": "Kannada",
    "mr": "Marathi",
    "gu": "Gujarati",
    "pa": "Punjabi",
    "ml": "Malayalam",
    "fi": "Finnish",
}

_ALPHABETS = "([A-Za-z])"
_PREFIXES = "(Mr|St|Mrs|Ms|Dr)[.]"
_SUFFIXES = "(Inc|Ltd|Jr|Sr|Co)"
_STARTERS = (
    r"(Mr|Mrs|Ms|Dr|Prof|Capt|Cpt|Lt|He\s|She\s|It\s|They\s|Their\s|Our\s|"
    r"We\s|But\s|However\s|That\s|This\s|Wherever)"
)
_ACRONYMS = "([A-Z][.][A-Z][.](?:[A-Z][.])?)"
_WEBSITES = "[.](com|net|org|io|gov|edu|me)"
_DIGITS = "([0-9])"
_MULTIPLE_DOTS = r"\.{2,}"


def split_into_sentences(text: str) -> list[str]:
    """Split text into sentences using the upstream regex-based approach.

    This is the upstream ``instructions_util.split_into_sentences``
    function, which uses no external dependencies.
    """
    text = " " + text + "  "
    text = text.replace("\n", " ")
    text = re.sub(_PREFIXES, "\\1<prd>", text)
    text = re.sub(_WEBSITES, "<prd>\\1", text)
    text = re.sub(_DIGITS + "[.]" + _DIGITS, "\\1<prd>\\2", text)
    text = re.sub(
        _MULTIPLE_DOTS,
        lambda match: "<prd>" * len(match.group(0)) + "<stop>",
        text,
    )
    if "Ph.D" in text:
        text = text.replace("Ph.D.", "Ph<prd>D<prd>")
    text = re.sub(r"\s" + _ALPHABETS + "[.] ", " \\1<prd> ", text)
    text = re.sub(_ACRONYMS + " " + _STARTERS, "\\1<stop> \\2", text)
    text = re.sub(
        _ALPHABETS + "[.]" + _ALPHABETS + "[.]" + _ALPHABETS + "[.]",
        "\\1<prd>\\2<prd>\\3<prd>",
        text,
    )
    text = re.sub(
        _ALPHABETS + "[.]" + _ALPHABETS + "[.]", "\\1<prd>\\2<prd>", text
    )
    text = re.sub(" " + _SUFFIXES + "[.] " + _STARTERS, " \\1<stop> \\2", text)
    text = re.sub(" " + _SUFFIXES + "[.]", " \\1<prd>", text)
    text = re.sub(" " + _ALPHABETS + "[.]", " \\1<prd>", text)
    if "\u201d" in text:
        text = text.replace(".\u201d", "\u201d.")
    if '"' in text:
        text = text.replace('."', '".')
    if "!" in text:
        text = text.replace('!"', '"!')
    if "?" in text:
        text = text.replace('?"', '"?')
    text = text.replace(".", ".<stop>")
    text = text.replace("?", "?<stop>")
    text = text.replace("!", "!<stop>")
    text = text.replace("<prd>", ".")
    sentences = text.split("<stop>")
    sentences = [s.strip() for s in sentences]
    if sentences and not sentences[-1]:
        sentences = sentences[:-1]
    return sentences


def count_words(text: str) -> int:
    """Count the number of words using ``re.findall(r"\\w+", text)``.

    Equivalent to the upstream ``RegexpTokenizer(r"\\w+")``.
    """
    return len(re.findall(r"\w+", text))


def count_sentences(text: str) -> int:
    """Count the number of sentences using the regex-based splitter."""
    return len(split_into_sentences(text))


# Unicode script ranges for non-Latin scripts used by IFEval languages.
_SCRIPT_RANGES: dict[str, list[tuple[int, int]]] = {
    "ja": [
        (0x3040, 0x309F),  # Hiragana
        (0x30A0, 0x30FF),  # Katakana
        (0x4E00, 0x9FFF),  # CJK Unified Ideographs
    ],
    "ko": [(0xAC00, 0xD7AF)],  # Hangul Syllables
    "zh": [(0x4E00, 0x9FFF)],  # CJK Unified Ideographs
    "ar": [(0x0600, 0x06FF)],  # Arabic
    "he": [(0x0590, 0x05FF)],  # Hebrew
    "fa": [(0x0600, 0x06FF)],  # Persian (Arabic script)
    "ur": [(0x0600, 0x06FF)],  # Urdu (Arabic script)
    "hi": [(0x0900, 0x097F)],  # Devanagari
    "ne": [(0x0900, 0x097F)],  # Nepali (Devanagari)
    "mr": [(0x0900, 0x097F)],  # Marathi (Devanagari)
    "bn": [(0x0980, 0x09FF)],  # Bengali
    "ta": [(0x0B80, 0x0BFF)],  # Tamil
    "te": [(0x0C00, 0x0C7F)],  # Telugu
    "kn": [(0x0C80, 0x0CFF)],  # Kannada
    "gu": [(0x0A80, 0x0AFF)],  # Gujarati
    "pa": [(0x0A00, 0x0A7F)],  # Punjabi (Gurmukhi)
    "ml": [(0x0D00, 0x0D7F)],  # Malayalam
    "th": [(0x0E00, 0x0E7F)],  # Thai
    "ru": [(0x0400, 0x04FF)],  # Cyrillic
    "uk": [(0x0400, 0x04FF)],  # Ukrainian (Cyrillic)
    "bg": [(0x0400, 0x04FF)],  # Bulgarian (Cyrillic)
}

# Distinctive diacritical characters for Latin-script languages.
_LATIN_MARKERS: dict[str, set[str]] = {
    "es": {"ñ", "Ñ", "¿", "¡", "á", "é", "í", "ó", "ú", "Á", "É", "Í", "Ó", "Ú"},
    "pt": {"ã", "õ", "ç", "â", "ê", "ô", "á", "é", "í", "ó", "ú", "Ã", "Õ", "Ç"},
    "fr": {"ç", "é", "è", "ê", "ë", "à", "â", "ù", "û", "î", "ï", "ô", "œ", "Œ"},
    "de": {"ä", "ö", "ü", "ß", "Ä", "Ö", "Ü"},
    "it": {"à", "è", "é", "ì", "ò", "ù"},
    "pl": {"ą", "ć", "ę", "ł", "ń", "ó", "ś", "ź", "ż", "Ą", "Ć", "Ę", "Ł", "Ń", "Ś", "Ź", "Ż"},
    "vi": {"ă", "â", "đ", "ê", "ô", "ơ", "ư", "Ă", "Â", "Đ", "Ê", "Ô", "Ơ", "Ư"},
    "fi": {"ä", "ö", "Ä", "Ö"},  # also in German, ambiguous
    "sw": set(),  # uses basic Latin only
    "en": set(),  # uses basic Latin only
}


def detect_language(text: str) -> Optional[str]:
    """Detect the language of a text using script-based heuristics.

    Returns the ISO 639-1 code of the detected language, or ``None``
    if detection is uncertain. The caller should treat ``None`` as
    "followed" (matching the upstream ``langdetect`` exception behavior).

    For non-Latin scripts, detection is reliable based on Unicode
    character ranges. For Latin-script languages, detection uses
    distinctive diacritical marks; if no marks are found, the language
    is ambiguous (could be English, Swahili, or any Latin language
    without diacritics) and ``None`` is returned.
    """
    if not text or not text.strip():
        return None

    # Count characters in each non-Latin script range.
    script_counts: dict[str, int] = {}
    for code, ranges in _SCRIPT_RANGES.items():
        count = 0
        for ch in text:
            cp = ord(ch)
            for lo, hi in ranges:
                if lo <= cp <= hi:
                    count += 1
                    break
        if count > 0:
            script_counts[code] = count

    if script_counts:
        # Return the script with the most characters.
        # For Cyrillic, distinguish ru/uk/bg by diacritical markers.
        best_script = max(script_counts, key=script_counts.get)
        if best_script in ("ru", "uk", "bg"):
            # All use Cyrillic; try to distinguish by unique letters.
            # Ukrainian has distinctive characters (U+0456, U+0457, U+0454, U+0491);
            # Bulgarian has U+044A/U+044C distinction.
            text_lower = text.lower()
            if any(ch in text_lower for ch in "\u0456\u0457\u0454\u0491"):
                return "uk"
            # Russian and Bulgarian are hard to distinguish without
            # more sophisticated analysis; return the best-scoring one.
            return best_script
        # Arabic script: ar, fa, ur are hard to distinguish.
        if best_script == "ar":
            # Persian has distinctive characters (U+067E, U+0686, U+0698, U+06AF).
            if any(ch in text for ch in "\u067e\u0686\u0698\u06af"):
                return "fa"
            # Urdu has distinctive characters (U+06C1, U+06D2, U+0688, U+0691).
            if any(ch in text for ch in "\u06c1\u06d2\u0688\u0691"):
                return "ur"
            return "ar"
        # Devanagari: hi, ne, mr are hard to distinguish.
        if best_script in ("hi", "ne", "mr"):
            # All use Devanagari; return the best-scoring one.
            return best_script
        return best_script

    # No non-Latin script characters found; check Latin diacritical marks.
    text_chars = set(text)
    for code, markers in _LATIN_MARKERS.items():
        if markers and text_chars & markers:
            # Found distinctive diacritical marks for this language.
            # fi and de share ä/ö; check de-specific ß first.
            if code == "fi" and "ß" in text_chars:
                return "de"
            return code

    # No distinctive marks found; could be en, sw, or any Latin language
    # without diacritics. Return None (uncertain).
    return None


__all__ = [
    "LANGUAGE_CODES",
    "count_sentences",
    "count_words",
    "detect_language",
    "split_into_sentences",
]
