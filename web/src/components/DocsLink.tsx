import { Link, useLocation } from "react-router-dom";
import { useT } from "../i18n";

export default function DocsLink() {
  const { t } = useT();
  const location = useLocation();
  return (
    <Link className="docs-link" to="/docs" state={{ from: location.pathname }} aria-label={t("docs.nav")}>
      <span className="docs-link-mark" aria-hidden="true">?</span>
      <span className="docs-link-label">{t("docs.nav")}</span>
    </Link>
  );
}
