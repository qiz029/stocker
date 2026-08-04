export type MobileNavItem = {
  id: string;
  label: string;
  icon: string;
  onSelect: () => void;
  primary?: boolean;
  ariaLabel?: string;
};

export default function MobileNav({
  label,
  active,
  items,
}: {
  label: string;
  active: string;
  items: MobileNavItem[];
}) {
  return (
    <nav className="mobile-nav" aria-label={label}>
      <div className="mobile-nav-inner">
        {items.map(item => (
          <button
            type="button"
            key={item.id}
            className={`mobile-nav-item ${item.primary ? "primary" : ""} ${active === item.id ? "on" : ""}`}
            aria-current={active === item.id ? "page" : undefined}
            aria-label={item.ariaLabel}
            onClick={item.onSelect}
          >
            <span className="mobile-nav-icon" aria-hidden="true">{item.icon}</span>
            <span>{item.label}</span>
          </button>
        ))}
      </div>
    </nav>
  );
}

export function scrollToMobileSection(id: string) {
  const element = document.getElementById(id);
  if (typeof element?.scrollIntoView === "function") {
    element.scrollIntoView({ behavior: "smooth", block: "start" });
  }
}
