package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *app) startVocabReview() {
	due := GetVocabularyStore().DueEntries(time.Now())
	ids := make([]string, 0, len(due))
	for _, e := range due {
		ids = append(ids, e.ID)
	}
	a.vocabReviewIDs = ids
	a.vocabReviewIdx = 0
	a.vocabReviewRevealed = false
	a.vocabReviewDone = 0
	a.vocabReviewing = true
	a.settingsRefresh()
}

func (a *app) endVocabReview() {
	a.vocabReviewing = false
	a.vocabReviewIDs = nil
	a.vocabReviewIdx = 0
	a.vocabReviewRevealed = false
	a.settingsRefresh()
}

func (a *app) openVocabReview() {
	a.page = "vocabulary"
	a.showSettings()
	a.startVocabReview()
}

func (a *app) gradeVocabReview(grade reviewGrade) {
	if a.vocabReviewIdx >= len(a.vocabReviewIDs) {
		return
	}
	id := a.vocabReviewIDs[a.vocabReviewIdx]
	if _, ok := GetVocabularyStore().ApplyReview(id, grade, time.Now()); ok {
		a.vocabReviewDone++
	}
	a.vocabReviewIdx++
	a.vocabReviewRevealed = false
	a.settingsRefresh()
}

func (a *app) pageVocabReview(c *ui.Context, t *ui.Theme) {
	total := len(a.vocabReviewIDs)
	if a.vocabReviewIdx >= total {
		a.reviewSummary(c, t)
		return
	}

	store := GetVocabularyStore()
	var entry VocabularyEntry
	found := false
	for a.vocabReviewIdx < total {
		if e, ok := store.Get(a.vocabReviewIDs[a.vocabReviewIdx]); ok {
			entry, found = e, true
			break
		}
		a.vocabReviewIdx++
		a.vocabReviewRevealed = false
	}
	if !found {
		a.reviewSummary(c, t)
		return
	}

	if c.Shortcut(0, ui.KeyR) {
		PlaySpeechLang(entry.Term, "", false)
	}
	if !a.vocabReviewRevealed {
		if c.Shortcut(0, ui.KeySpace) || c.Shortcut(0, ui.KeyEnter) {
			a.vocabReviewRevealed = true
			a.settingsRefresh()
		}
	} else if c.Shortcut(0, ui.Key1) {
		a.gradeVocabReview(reviewAgain)
	} else if c.Shortcut(0, ui.Key2) {
		a.gradeVocabReview(reviewHard)
	} else if c.Shortcut(0, ui.Key3) {
		a.gradeVocabReview(reviewGood)
	}

	PageTitle(c, t, "生词复习", "先回忆，再显示答案。三个按钮决定这个词下次出现的时间。")
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Progress(c, float64(a.vocabReviewIdx)/float64(total)).Grow(1)
		ui.Textf(c, "剩余 %d / %d", total-a.vocabReviewIdx, total).FontSize(12).TextColor(t.TextMuted)
	})

	at := appTheme(t)
	ui.Box(c).FillWidth().Padding(28, 26).Radius(RadiusTrack).Background(at.Surface).
		Border(1, at.BorderSubtle).Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, entry.Term).Font(fontHeadword).FontSize(28).FontWeight(700).TextColor(at.Text).Wrap()
			ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
				if entry.Phonetic != "" {
					ui.Text(c, entry.Phonetic).Font(fontIPA).FontSize(FontIPASize).TextColor(at.TextMuted)
				}
				if renderIconButton(c, iconSpeak, "朗读发音", 24, 13, false).Clicked() {
					PlaySpeechLang(entry.Term, "", false)
				}
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if entry.PartOfSpeech != "" {
					ui.Box(c).Padding(1, 6).Radius(4).Background(t.SurfaceHover).Children(func() {
						ui.Text(c, entry.PartOfSpeech).FontSize(11).FontWeight(600).TextColor(t.Accent)
					})
				}
				if entry.Difficulty != "" {
					d := entry.Difficulty
					bg, fg := cefrColors(t, d)
					ui.Box(c).Padding(1, 6).Radius(10).Background(bg).Children(func() {
						ui.Text(c, d).FontSize(10).FontWeight(700).TextColor(fg)
					})
				}
			})

			if !a.vocabReviewRevealed {
				ui.Text(c, "回想它的意思，然后显示答案。").FontSize(12).TextColor(t.TextMuted)
				if PrimaryButton(c, t, "显示答案").Clicked() {
					a.vocabReviewRevealed = true
					a.settingsRefresh()
				}
				return
			}

			ui.Divider(c).FillWidth()
			if entry.Definition != "" {
				ui.Text(c, entry.Definition).FontSize(15).TextColor(t.Text).Wrap()
			} else {
				ui.Text(c, "这个词没有保存释义。").FontSize(13).TextColor(t.TextMuted)
			}
			if entry.Context != "" {
				ui.Text(c, entry.Context).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
			if entry.ContextTranslation != "" {
				ui.Text(c, entry.ContextTranslation).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
			if entry.MemoryTips != "" {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconInfo).Size(12, 12).TextColor(t.TextMuted)
					ui.Text(c, entry.MemoryTips).FontSize(12).TextColor(t.TextMuted).Wrap()
				})
			}

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if SecondaryButton(c, t, "忘记").Clicked() {
					a.gradeVocabReview(reviewAgain)
				}
				if SecondaryButton(c, t, "模糊").Clicked() {
					a.gradeVocabReview(reviewHard)
				}
				if PrimaryButton(c, t, "记得").Clicked() {
					a.gradeVocabReview(reviewGood)
				}
			})
			ui.Text(c, "忘记 (1)：10 分钟后 · 模糊 (2)：间隔小幅延长 · 记得 (3)：间隔翻倍 · R 朗读").FontSize(11).TextColor(t.TextMuted)
		})
	})

	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		if GhostButton(c, t, "结束本轮复习").Height(HeightCompact).Clicked() {
			a.endVocabReview()
		}
	})
}

func (a *app) reviewSummary(c *ui.Context, t *ui.Theme) {
	at := appTheme(t)
	PageTitle(c, t, "复习完成", fmt.Sprintf("本轮复习了 %d 个词。到期的词会重新回到这里。", a.vocabReviewDone))
	ui.Box(c).FillWidth().Padding(32, 24).Radius(RadiusTrack).Background(at.Surface).Border(1, at.BorderSubtle).Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			if a.vocabReviewDone == 0 && len(a.vocabReviewIDs) == 0 {
				ui.Text(c, "今天没有到期的生词。").FontSize(15).FontWeight(600).TextColor(at.Text)
				ui.Text(c, "新保存的词会立即进入复习队列；已经复习过的词会按间隔回来。").FontSize(12).TextColor(at.TextMuted).Wrap()
			} else {
				ui.Box(c).Size(40, 40).Radius(RadiusTrack).Center().Background(at.SuccessSoft).Children(func() {
					ui.Icon(c, iconCheck).Size(20, 20).TextColor(at.Success)
				})
				ui.Text(c, "今天的词复习完了").FontSize(16).FontWeight(600).TextColor(at.Text)
			}
			ui.Row(c).Gap(8).Children(func() {
				if PrimaryButton(c, t, "再来一轮").Height(HeightCompact).Clicked() {
					a.startVocabReview()
				}
				if SecondaryButton(c, t, "返回生词本").Height(HeightCompact).Clicked() {
					a.endVocabReview()
				}
			})
		})
	})
}
