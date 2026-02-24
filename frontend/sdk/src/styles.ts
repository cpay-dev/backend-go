export const CSS = `
.cpay-root{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#18181b;line-height:1.5;box-sizing:border-box}
.cpay-root *,.cpay-root *::before,.cpay-root *::after{box-sizing:inherit}
.cpay-card{border:1px solid #e4e4e7;border-radius:12px;background:#fff;overflow:hidden;max-width:400px}
.cpay-img{width:100%;height:160px;object-fit:cover;display:block}
.cpay-body{padding:20px}
.cpay-title{margin:0 0 4px;font-size:18px;font-weight:600}
.cpay-desc{margin:0 0 16px;font-size:14px;color:#71717a}
.cpay-price-box{background:#fafafa;border:1px solid #e4e4e7;border-radius:8px;padding:16px;text-align:center;margin-bottom:16px}
.cpay-price{font-size:28px;font-weight:700;margin:0}
.cpay-price-sub{font-size:13px;color:#71717a;margin:4px 0 0}
.cpay-label{display:block;font-size:13px;font-weight:500;margin-bottom:6px}
.cpay-input{width:100%;padding:8px 12px;border:1px solid #e4e4e7;border-radius:8px;font-size:14px;outline:none;font-family:inherit;background:#fff;color:#18181b}
.cpay-input:focus{border-color:#18181b;box-shadow:0 0 0 2px rgba(24,24,27,.1)}
.cpay-group{margin-bottom:16px}
.cpay-btn{display:flex;align-items:center;justify-content:center;gap:8px;width:100%;padding:10px 16px;border:none;border-radius:8px;font-size:14px;font-weight:500;cursor:pointer;font-family:inherit;transition:background .15s}
.cpay-btn-primary{background:#18181b;color:#fff}
.cpay-btn-primary:hover{background:#27272a}
.cpay-btn-primary:disabled{background:#a1a1aa;cursor:not-allowed}
.cpay-btn-outline{background:transparent;border:1px solid #e4e4e7;color:#18181b}
.cpay-btn-outline:hover{background:#fafafa}
.cpay-btn-sm{padding:6px 12px;font-size:13px;width:auto;display:inline-flex}
.cpay-wallets{display:flex;flex-direction:column;gap:8px;margin-bottom:4px}
.cpay-wallet-btn{display:flex;align-items:center;gap:10px;width:100%;padding:10px 14px;border:1px solid #e4e4e7;border-radius:8px;background:#fff;font-size:14px;cursor:pointer;font-family:inherit;transition:border-color .15s,background .15s}
.cpay-wallet-btn:hover{border-color:#a1a1aa;background:#fafafa}
.cpay-wallet-icon{width:24px;height:24px;border-radius:6px}
.cpay-wallet-name{font-weight:500}
.cpay-addr{text-align:center;font-size:12px;color:#a1a1aa;margin-top:12px}
.cpay-err{text-align:center;font-size:13px;color:#dc2626;margin-bottom:12px}
.cpay-ok{text-align:center;padding:32px 20px}
.cpay-ok-icon{font-size:48px;margin-bottom:8px}
.cpay-ok-title{font-size:20px;font-weight:700;margin:0 0 4px}
.cpay-ok-sub{font-size:14px;color:#71717a;margin:0}
.cpay-methods{display:flex;flex-direction:column;gap:8px;margin-bottom:16px}
.cpay-method{border:1px solid #e4e4e7;border-radius:8px;padding:12px;cursor:pointer;transition:border-color .15s,background .15s}
.cpay-method:hover{background:#fafafa}
.cpay-method.cpay-selected{border-color:#18181b;background:#fafafa}
.cpay-method-title{font-weight:500;font-size:14px;margin:0 0 2px}
.cpay-method-desc{font-size:13px;color:#71717a;margin:0}
.cpay-periods{display:flex;gap:6px;flex-wrap:wrap;margin-top:8px}
.cpay-period-btn{padding:4px 10px;border:1px solid #e4e4e7;border-radius:6px;background:#fff;font-size:12px;cursor:pointer;font-family:inherit}
.cpay-period-btn.cpay-selected{background:#18181b;color:#fff;border-color:#18181b}
.cpay-back{display:inline-flex;align-items:center;gap:4px;font-size:13px;color:#71717a;cursor:pointer;border:none;background:none;padding:0;margin-bottom:12px;font-family:inherit}
.cpay-back:hover{color:#18181b}
.cpay-sub-info{background:#fafafa;border:1px solid #e4e4e7;border-radius:8px;padding:12px;font-size:13px;margin-bottom:16px}
.cpay-sub-row{display:flex;justify-content:space-between;padding:2px 0}
.cpay-sub-label{color:#71717a}
.cpay-badge{display:inline-block;padding:2px 8px;border-radius:9999px;font-size:11px;font-weight:500;text-transform:capitalize}
.cpay-badge-ok{background:#dcfce7;color:#166534}
.cpay-badge-muted{background:#f4f4f5;color:#71717a}
.cpay-btn-danger{background:#dc2626;color:#fff}
.cpay-btn-danger:hover{background:#b91c1c}
.cpay-powered{text-align:center;margin-top:16px;font-size:11px;color:#a1a1aa}
.cpay-powered a{color:#71717a;text-decoration:none}
.cpay-powered a:hover{text-decoration:underline}
.cpay-dark .cpay-card{background:#18181b;border-color:#27272a;color:#fafafa}
.cpay-dark .cpay-input{background:#27272a;border-color:#3f3f46;color:#fafafa}
.cpay-dark .cpay-price-box,.cpay-dark .cpay-sub-info{background:#27272a;border-color:#3f3f46}
.cpay-dark .cpay-method{border-color:#3f3f46}
.cpay-dark .cpay-method:hover,.cpay-dark .cpay-method.cpay-selected{background:#27272a}
.cpay-dark .cpay-wallet-btn{background:#27272a;border-color:#3f3f46;color:#fafafa}
.cpay-dark .cpay-wallet-btn:hover{background:#3f3f46;border-color:#52525b}
`;

let injected = false;
export function injectStyles() {
  if (injected) return;
  injected = true;
  const el = document.createElement("style");
  el.id = "cpay-sdk-styles";
  el.textContent = CSS;
  document.head.appendChild(el);
}
