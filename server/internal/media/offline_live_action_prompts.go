package media

import "strings"

// ResolveOfflineLiveActionPromptFromTitle maps auto-generated offline video titles
// (动作-* / 待机-*) to LiveAct prompts when the client omitted actionPrompt.
func ResolveOfflineLiveActionPromptFromTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if prompt, ok := offlineLiveTitlePrompts[title]; ok {
		return prompt
	}
	return ""
}

//nolint:lll // Prompt strings are copied from frontend zh-CN i18n for server-side fallback.
var offlineLiveTitlePrompts = map[string]string{
	"动作-喝水":   "面向镜头，先说话，再拿出白色杯子喝水，喝完放下杯子看向镜头。",
	"动作-戴帽子":  "面向镜头，先说话，再拿出浅米色帽子戴上，整理一下，戴好后看向镜头。",
	"动作-比心":   "面向镜头，先说话，再在胸前比爱心，比完面向镜头微笑。",
	"动作-比耶":   "面向镜头，先说话，再抬起手比耶，比完放下手看向镜头。",
	"动作-吃西瓜":  "面向镜头，先说话，再拿出西瓜片吃一口，吃完看向镜头微笑。",
	"动作-歪头卖萌": "面向镜头，先说话，再轻轻歪头卖萌，回正看向镜头。",
	"动作-闭上单眼卖萌": "面向镜头，先说话，再单眼眨一下卖萌，再看向镜头微笑。",
	"动作-撒娇":   "面向镜头，先说话，再双手放胸前微微嘟嘴撒娇，然后看向镜头。",
	"动作-小情绪生气": "面向镜头，先说话，再略生气噘嘴，缓和后面向镜头。",
	"动作-假装生气":  "面向镜头，先说话，再略生气噘嘴，缓和后面向镜头。",
	"待机-正常播报": "一个人面对镜头，半身构图，自然眨眼，偶尔轻轻点头，安静待机。",
	"待机-左右看看": "一个人面对镜头，半身构图，慢慢看左、看右，再回到镜头，轻微笑。",
	"待机-犯困打盹": "一个人面对镜头，半身构图，有点犯困，眨眼变慢，小哈欠一次，头微点。",
	"待机-偶尔卖萌": "一个人面对镜头，半身构图，多数时间安静看镜头，偶尔歪头或眨眼卖萌。",
	"待机-放松呼吸": "一个人面对镜头，半身构图，柔和呼吸，慢眨眼，动作很小。",
	"待机-整理仪容": "一个人面对镜头，半身构图，拨一下头发或理衣领，再看回镜头。",
	"待机-托腮放空": "一个人面对镜头，半身构图，托腮发呆一下，再看回镜头微笑。",
	"待机-浅笑陪伴": "一个人面对镜头，半身构图，浅笑看镜头，自然眨眼，偶尔轻点头。",
}
