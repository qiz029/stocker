import Sparkline from "./Sparkline";

type Props = {
  name: string;
  sub: string;
  price: string;
  pill: string;
  pillUp: boolean;
  sparkSeries?: number[];
  onClick?: () => void;
};

export default function InstrumentRow({ name, sub, price, pill, pillUp, sparkSeries, onClick }: Props) {
  return (
    <div className="row" onClick={onClick} style={onClick ? undefined : { cursor: "default" }}>
      <div>
        <div className="name">{name}</div>
        <div className="desc">{sub}</div>
      </div>
      <div>{sparkSeries && sparkSeries.length > 1 ? <Sparkline series={sparkSeries} /> : null}</div>
      <div className="px">
        <div className="p num">{price}</div>
        {pill && <span className={`pill num ${pillUp ? "up" : "down"}`}>{pill}</span>}
      </div>
    </div>
  );
}
