import { FormEvent, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, ApiError, AvatarID, SocialLinkKey, User } from "../api";
import { useUpdateUser, useUser } from "../App";
import { avatarGlyph, avatarGlyphs, avatarIDs } from "../avatar";
import DocsLink from "../components/DocsLink";
import { hallMockEnabled } from "../devHallFixtures";
import { LangSwitch, useT } from "../i18n";
import { useToast } from "../Toast";
import "./Profile.css";

const profileCopy = {
  en: {
    back: "Back to Market Hall", eyebrow: "ACCOUNT / IDENTITY", title: "Your profile",
    sub: "Shape how other players see you across every timeline.", preview: "Public preview", member: "Market participant",
    identity: "Identity", identitySub: "Your public name, avatar, and introduction.", alias: "Alias", aliasHint: "Shown in rooms and rankings",
    email: "Email", emailHint: "Private · never shown to other players", description: "Description", descriptionPlaceholder: "Share your investing style, interests, or a little about yourself…",
    avatar: "Choose an avatar", social: "Social links", socialSub: "Optional links shown on your public profile.", website: "Website", x: "X / Twitter", github: "GitHub", linkedin: "LinkedIn",
    save: "Save profile", saving: "Saving…", saved: "Profile saved", noDescription: "Add a description to introduce yourself.",
    security: "Security", securitySub: "Use a strong password you do not reuse elsewhere.", currentPassword: "Current password",
    newPassword: "New password", confirmPassword: "Confirm new password", updatePassword: "Update password", updatingPassword: "Updating…",
    passwordUpdated: "Password updated", passwordMismatch: "New passwords do not match.", mockNotice: "Mock preview updated locally.",
    saveFailed: "Could not save profile", passwordFailed: "Could not update password",
  },
  zh: {
    back: "返回时代大厅", eyebrow: "账户 / 身份", title: "个人资料",
    sub: "管理你在每条时间线中展示给其他玩家的身份。", preview: "公开预览", member: "市场参与者",
    identity: "身份资料", identitySub: "你的公开昵称、头像与个人介绍。", alias: "Alias / 昵称", aliasHint: "显示在房间和排行榜中",
    email: "邮箱", emailHint: "仅自己可见 · 不向其他玩家展示", description: "个人介绍", descriptionPlaceholder: "介绍一下你的投资风格、兴趣，或者你自己……",
    avatar: "选择头像", social: "社交链接", socialSub: "可选，将展示在你的公开资料中。", website: "个人网站", x: "X / Twitter", github: "GitHub", linkedin: "LinkedIn",
    save: "保存资料", saving: "保存中…", saved: "个人资料已保存", noDescription: "添加一段介绍，让大家认识你。",
    security: "安全设置", securitySub: "请使用未在其他网站重复使用的强密码。", currentPassword: "当前密码",
    newPassword: "新密码", confirmPassword: "确认新密码", updatePassword: "更新密码", updatingPassword: "更新中…",
    passwordUpdated: "密码已更新", passwordMismatch: "两次输入的新密码不一致。", mockNotice: "Mock 预览已在本地更新。",
    saveFailed: "保存个人资料失败", passwordFailed: "更新密码失败",
  },
};

const socialNetworks: { key: SocialLinkKey; icon: string }[] = [
  { key: "website", icon: "⌁" }, { key: "x", icon: "𝕏" }, { key: "github", icon: "◈" }, { key: "linkedin", icon: "in" },
];

function isSafePreviewLink(value: string) {
  return /^https?:\/\//i.test(value.trim());
}

export default function Profile() {
  const account = useUser();
  const updateAccount = useUpdateUser();
  const navigate = useNavigate();
  const { lang } = useT();
  const c = profileCopy[lang];
  const { toast, node } = useToast();
  const mockHall = hallMockEnabled();
  const hallPath = mockHall ? "/?mock=hall" : "/";
  const [alias, setAlias] = useState(account.display_name ?? "");
  const [avatarID, setAvatarID] = useState<AvatarID>(account.avatar_id ?? "bull");
  const [email, setEmail] = useState(account.email ?? "");
  const [description, setDescription] = useState(account.description ?? "");
  const [links, setLinks] = useState<Record<SocialLinkKey, string>>({
    website: account.social_links?.website ?? "", x: account.social_links?.x ?? "",
    github: account.social_links?.github ?? "", linkedin: account.social_links?.linkedin ?? "",
  });
  const [saving, setSaving] = useState(false);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordBusy, setPasswordBusy] = useState(false);
  const [passwordError, setPasswordError] = useState("");

  const activeLinks = useMemo(() => socialNetworks.filter(({ key }) => isSafePreviewLink(links[key])), [links]);

  async function saveProfile(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    const socialLinks = Object.fromEntries(Object.entries(links).filter(([, value]) => value.trim()).map(([key, value]) => [key, value.trim()]));
    try {
      const updated: User = mockHall
        ? { ...account, display_name: alias.trim(), avatar_id: avatarID, email: email.trim(), description: description.trim(), social_links: socialLinks, profile_complete: true }
        : await api.put<User>("/api/me/profile", { display_name: alias.trim(), avatar_id: avatarID, email: email.trim(), description: description.trim(), social_links: socialLinks });
      updateAccount(updated);
      toast(mockHall ? c.mockNotice : c.saved);
    } catch (err) {
      toast(err instanceof ApiError ? err.message : c.saveFailed);
    } finally {
      setSaving(false);
    }
  }

  async function changePassword(event: FormEvent) {
    event.preventDefault();
    setPasswordError("");
    if (newPassword !== confirmPassword) {
      setPasswordError(c.passwordMismatch);
      return;
    }
    setPasswordBusy(true);
    try {
      if (!mockHall) await api.put("/api/me/password", { current_password: currentPassword, new_password: newPassword });
      setCurrentPassword(""); setNewPassword(""); setConfirmPassword("");
      toast(mockHall ? c.mockNotice : c.passwordUpdated);
    } catch (err) {
      setPasswordError(err instanceof ApiError ? err.message : c.passwordFailed);
    } finally {
      setPasswordBusy(false);
    }
  }

  return <div className="profile-page">
    <header className="topbar profile-topbar">
      <button type="button" className="brand profile-brand" onClick={()=>navigate(hallPath)}><em>●</em> Stocker</button>
      <div className="spacer"/><DocsLink/><LangSwitch/>
      <div className="profile-top-account"><span className="avatar">{avatarGlyph(avatarID, account.username)}</span><span>{account.username}</span></div>
    </header>
    <main className="profile-shell">
      <button type="button" className="profile-back" aria-label={c.back} onClick={()=>navigate(hallPath)}>← {c.back}</button>
      <section className="profile-hero"><span>{c.eyebrow}</span><h1>{c.title}</h1><p>{c.sub}</p></section>
      <div className="profile-layout">
        <aside className="profile-preview">
          <div className="profile-preview-grid" aria-hidden="true"/><span className="profile-preview-label">{c.preview}</span>
          <div className="profile-preview-avatar">{avatarGlyph(avatarID, account.username)}</div>
          <h2>{alias.trim() || account.username}</h2><p className="profile-handle">@{account.username}</p>
          <p className={`profile-preview-description ${description.trim()?"":"empty"}`}>{description.trim() || c.noDescription}</p>
          <div className="profile-preview-links">{activeLinks.map(({key,icon})=><a href={links[key]} target="_blank" rel="noreferrer" aria-label={`${c[key]} link`} key={key}>{icon}</a>)}</div>
          <div className="profile-member"><i/><span>{c.member}</span></div>
        </aside>
        <div className="profile-content">
          <form className="profile-card" onSubmit={saveProfile}>
            <header><span className="profile-section-number">01</span><div><h2>{c.identity}</h2><p>{c.identitySub}</p></div></header>
            <div className="profile-field-grid">
              <label><span>{c.alias}<small>{c.aliasHint}</small></span><input aria-label={c.alias} minLength={2} maxLength={24} required value={alias} onChange={event=>setAlias(event.target.value)}/></label>
              <label><span>{c.email}<small>{c.emailHint}</small></span><input aria-label={c.email} type="email" maxLength={254} value={email} onChange={event=>setEmail(event.target.value)}/></label>
            </div>
            <fieldset className="profile-avatar-field"><legend>{c.avatar}</legend><div>{avatarIDs.map(id=><button type="button" aria-label={id} aria-pressed={avatarID===id} className={avatarID===id?"selected":""} onClick={()=>setAvatarID(id)} key={id}>{avatarGlyphs[id]}</button>)}</div></fieldset>
            <label className="profile-description-field"><span>{c.description}<small>{description.length}/500</small></span><textarea aria-label={c.description} maxLength={500} rows={4} placeholder={c.descriptionPlaceholder} value={description} onChange={event=>setDescription(event.target.value)}/></label>
            <div className="profile-social"><div><h3>{c.social}</h3><p>{c.socialSub}</p></div><div className="profile-social-grid">{socialNetworks.map(({key,icon})=><label key={key}><span><i>{icon}</i>{c[key]}</span><input aria-label={c[key]} type="url" maxLength={200} placeholder="https://" value={links[key]} onChange={event=>setLinks(current=>({...current,[key]:event.target.value}))}/></label>)}</div></div>
            <footer><button className="profile-save" disabled={saving||alias.trim().length<2}>{saving?c.saving:c.save}</button></footer>
          </form>
          <form className="profile-card profile-security" onSubmit={changePassword}>
            <header><span className="profile-section-number">02</span><div><h2>{c.security}</h2><p>{c.securitySub}</p></div></header>
            <div className="profile-password-grid">
              <label><span>{c.currentPassword}</span><input aria-label={c.currentPassword} type="password" autoComplete="current-password" required value={currentPassword} onChange={event=>setCurrentPassword(event.target.value)}/></label>
              <label><span>{c.newPassword}</span><input aria-label={c.newPassword} type="password" autoComplete="new-password" minLength={8} maxLength={72} required value={newPassword} onChange={event=>setNewPassword(event.target.value)}/></label>
              <label><span>{c.confirmPassword}</span><input aria-label={c.confirmPassword} type="password" autoComplete="new-password" minLength={8} maxLength={72} required value={confirmPassword} onChange={event=>setConfirmPassword(event.target.value)}/></label>
            </div>
            {passwordError&&<p className="profile-form-error">{passwordError}</p>}
            <footer><button className="profile-password-save" disabled={passwordBusy}>{passwordBusy?c.updatingPassword:c.updatePassword}</button></footer>
          </form>
        </div>
      </div>
    </main>
    {node}
  </div>;
}
