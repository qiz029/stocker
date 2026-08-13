import Sparkline from "./Sparkline";
import { useChangeFlash } from "../useChangeFlash";

type Props = {
  name: string;
  sub: string;
  price: string;
  pill: string;
  pillUp: boolean;
  pnl?: { text: string; up: boolean };   // avg cost + unrealized P&L line (held positions)
  sparkSeries?: number[];
  priceValue?: number;   // numeric quote behind `price`; flashes the price red/green on change
  sharesValue?: number;  // held shares; glows the row on change (order fill)
  onClick?: () => void;
};

export default function InstrumentRow({ name, sub, price, pill, pillUp, pnl, sparkSeries, priceValue, sharesValue, onClick }: Props) {
  const priceFlash = useChangeFlash(priceValue ?? "");
  const fillFlash = useChangeFlash(sharesValue ?? "");
  return (
    <div className="row" onClick={onClick} style={onClick ? undefined : { cursor: "default" }}>
      {fillFlash.nonce > 0 && <i key={fillFlash.nonce} className="row-glow" aria-hidden="true" />}
      <div>
        <div className="name">{name}</div>
        <div className="desc">{sub}</div>
      </div>
      <div>{sparkSeries && sparkSeries.length > 1 ? <Sparkline series={sparkSeries} /> : null}</div>
      <div className="px">
        <div
          key={priceFlash.nonce}
          className={`p num ${priceFlash.nonce ? (priceFlash.dir >= 0 ? "flash-up" : "flash-down") : ""}`}
        >{price}</div>
        {pill && <span className={`pill num ${pillUp ? "up" : "down"}`}>{pill}</span>}
        {pnl && <div className={`pnl delta num ${pnl.up ? "up" : "down"}`}>{pnl.text}</div>}
      </div>
    </div>
  );
}
