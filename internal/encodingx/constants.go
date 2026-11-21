package encodingx

const latinAdjacencyPenalty int64 = -50

const implausibilityPenalty int64 = -220

const ordinalBonus int64 = 300

// Must match the ISO-8859-2 score for " Š ".
const copyrightBonus int64 = 222

const implausibleLatinCaseTransitionPenalty int64 = -180

const nonLatinCapitalizationBonus int64 = 40

const nonLatinAllCapsPenalty int64 = -40

const nonLatinMixedCasePenalty int64 = -20

// Manually calibrated relative to windows-1256 Arabic
const cjkBaseScore int64 = 41

const cjkSecondaryBaseScore int64 = 20

const shiftJISScorePerKana int64 = 20

const shiftJISScorePerLevel1Kanji = cjkBaseScore

const shiftJISScorePerLevel2Kanji = cjkSecondaryBaseScore

// Manually calibrated relative to windows-1256 Persian and Urdu
const shiftJISInitialHalfWidthKatakanaPenalty int64 = -75

const halfWidthKatakanaScore int64 = 1

const halfWidthKatakanaVoicingScore int64 = 10

const shiftJISPUAPenalty = -(cjkBaseScore * 10)

const shiftJISExtensionPenalty = shiftJISPUAPenalty * 2

const shiftJISSingleByteExtensionPenalty = shiftJISExtensionPenalty

const eucJPScorePerKana = cjkBaseScore + (cjkBaseScore / 3) // Relative to Big5

const eucJPScorePerNearObsoleteKana = cjkBaseScore - 1

const eucJPScorePerLevel1Kanji = cjkBaseScore

const eucJPScorePerLevel2Kanji = cjkSecondaryBaseScore

const eucJPScorePerOtherKanji = cjkSecondaryBaseScore / 4

const eucJPInitialKanaPenalty = -((cjkBaseScore / 3) + 1)

// Needs to be more severe than for Shift_JIS to avoid misdetecting EUC-KR.
const eucJPExtensionPenalty = -(cjkBaseScore * 50)

const big5ScorePerLevel1Hanzi = cjkBaseScore

const big5ScorePerOtherHanzi = cjkSecondaryBaseScore

// More severe than other PUA penalties to avoid misdetecting EUC-KR.
const big5PUAPenalty = -(cjkBaseScore * 30)

const big5SingleByteExtensionPenalty = -(cjkBaseScore * 40)

const eucKRScorePerEUCHangul = cjkBaseScore + 1

const eucKRScorePerNonEUCHangul = cjkSecondaryBaseScore / 5

const eucKRScorePerHanja = cjkSecondaryBaseScore / 2

const eucKRHanjaAfterHangulPenalty = -(cjkBaseScore * 10)

const eucKRLongWordPenalty int64 = -6

const eucKRPUAPenalty = gbkPUAPenalty - 1 // Break tie in favor of GBK

const eucKRMacKoreanPenalty = eucKRPUAPenalty * 2

const eucKRSingleByteExtensionPenalty = eucKRMacKoreanPenalty

const gbkScorePerLevel1 = cjkBaseScore

const gbkScorePerLevel2 = cjkSecondaryBaseScore

const gbkScorePerNonEUC = cjkSecondaryBaseScore / 4

const gbkPUAPenalty = -(cjkBaseScore * 10)

const gbkSingleByteExtensionPenalty = gbkPUAPenalty * 4

const cjkLatinAdjacencyPenalty = -cjkBaseScore // smaller penalty than latinAdjacencyPenalty

const cjPunctuation = cjkBaseScore / 2

const cjkOther = cjkSecondaryBaseScore / 4

// Latin letter caseless class
const latinLetter uint8 = 1
