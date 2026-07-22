import * as echarts from "echarts/core";
import { BarChart, LineChart, PieChart, ScatterChart } from "echarts/charts";
import {
  DataZoomComponent,
  GeoComponent,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TooltipComponent,
} from "echarts/components";
import { SVGRenderer } from "echarts/renderers";
import worldMap from "./assets/world.geo.json";

echarts.use([
  LineChart,
  BarChart,
  PieChart,
  ScatterChart,
  DataZoomComponent,
  GeoComponent,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TooltipComponent,
  SVGRenderer,
]);
echarts.registerMap("visitortrace-world", worldMap);

const dataElement = document.getElementById("analytics-data");
const trendElement = document.getElementById("trend-chart-interactive");
const mapElement = document.getElementById("geo-chart");

if (dataElement && trendElement && mapElement) {
  document.body.classList.add("analytics-enhancing");
  try {
    const payload = JSON.parse(dataElement.textContent || "{}");
    const labels = payload.labels || {};
    const trend = echarts.init(trendElement, null, { renderer: "svg" });
    const map = echarts.init(mapElement, null, { renderer: "svg" });
    const charts = [trend, map];

    trend.setOption(trendOptions(payload.daily || [], labels, payload.rules || []));
    map.setOption(mapOptions(payload.points || [], labels));
    addDimensionChart(charts, "path-chart", barOptions(payload.paths || [], labels));
    addDimensionChart(charts, "browser-chart", pieOptions(payload.browsers || [], labels));
    addDimensionChart(charts, "os-chart", pieOptions(payload.operating_systems || [], labels));
    document.body.classList.remove("analytics-enhancing");
    document.body.classList.add("analytics-enhanced");

    const resize = () => {
      charts.forEach((chart) => chart.resize());
    };
    if ("ResizeObserver" in window) {
      const observer = new ResizeObserver(resize);
      observer.observe(trendElement);
      observer.observe(mapElement);
    } else {
      window.addEventListener("resize", resize, { passive: true });
    }
  } catch (error) {
    document.body.classList.remove("analytics-enhancing");
    console.warn("VisitorTrace analytics enhancement unavailable", error);
  }
}

function addDimensionChart(charts, id, options) {
  const element = document.getElementById(id);
  if (!element) return;
  const chart = echarts.init(element, null, { renderer: "svg" });
  chart.setOption(options);
  charts.push(chart);
}

function trendOptions(daily, labels, rules) {
  const byDate = new Map(daily.map((item) => [item.date, item]));
  const dates = [...new Set(daily.map((item) => item.date).concat(rules.map((rule) => rule.effective_date)))].sort();
  const pageviews = dates.map((date) => byDate.get(date)?.pageviews || 0);
  const visitors = dates.map((date) => byDate.get(date)?.unique_visitors || 0);
  return {
    animationDuration: 260,
    color: ["#d96e51", "#2f655a"],
    grid: { left: 48, right: 20, top: 42, bottom: daily.length > 30 ? 62 : 38 },
    legend: { top: 2, right: 8, itemWidth: 16, itemHeight: 3, textStyle: { color: "#526063" } },
    tooltip: { trigger: "axis", confine: true },
    xAxis: {
      type: "category",
      boundaryGap: false,
      data: dates,
      axisLine: { lineStyle: { color: "#dce4e3" } },
      axisLabel: { color: "#6e7c80", hideOverlap: true },
      axisTick: { show: false },
    },
    yAxis: {
      type: "value",
      minInterval: 1,
      axisLabel: { color: "#6e7c80" },
      splitLine: { lineStyle: { color: "#edf1ef" } },
    },
    dataZoom: daily.length > 30
      ? [
          { type: "inside", start: Math.max(0, 100 - (30 / daily.length) * 100), end: 100 },
          { type: "slider", height: 16, bottom: 8, borderColor: "#dce4e3", fillerColor: "rgba(47,101,90,.15)" },
        ]
      : [],
    series: [
      {
        name: labels.pageviews || "Pageviews",
        type: "line",
        data: pageviews,
        symbol: "circle",
        symbolSize: 5,
        lineStyle: { width: 2 },
        areaStyle: { opacity: 0.08 },
        markLine: rules.length ? {
          symbol: "none",
          lineStyle: { color: "#d9a942", type: "dashed", width: 1 },
          label: { color: "#765b10", formatter: (params) => `${params.data.windowDays} ${labels.days || "days"}` },
          data: rules.map((rule) => ({ xAxis: rule.effective_date, windowDays: rule.window_days })),
        } : undefined,
      },
      {
        name: labels.uniqueVisitors || "Unique Visitors",
        type: "line",
        data: visitors,
        symbol: "circle",
        symbolSize: 5,
        lineStyle: { width: 2 },
      },
    ],
  };
}

function mapOptions(points, labels) {
  const values = points.map((point) => ({
    name: point.name || labels.unknown || "Unknown",
    value: [point.lon < -170 ? point.lon + 360 : point.lon, point.lat, point.pv, point.uv],
  }));
  const maxPV = Math.max(1, ...points.map((point) => point.pv || 0));
  return {
    animationDuration: 260,
    tooltip: {
      trigger: "item",
      confine: true,
      formatter: (params) => {
        const value = params.value || [];
        return `${escapeHTML(params.name)}<br>${labels.pageviews || "Pageviews"}: ${value[2] || 0}<br>${labels.uniqueVisitors || "Unique Visitors"}: ${value[3] || 0}`;
      },
    },
    geo: {
      map: "visitortrace-world",
      roam: true,
      top: 6,
      bottom: 6,
      left: 2,
      right: 2,
      itemStyle: { areaColor: "#e7edeb", borderColor: "#aebdb9", borderWidth: 0.7 },
      emphasis: { itemStyle: { areaColor: "#dce8e3" }, label: { show: false } },
      select: { disabled: true },
    },
    series: [{
      name: labels.visitors || "Visitors",
      type: "scatter",
      coordinateSystem: "geo",
      data: values,
      symbolSize: (value) => 5 + Math.sqrt((value[2] || 0) / maxPV) * 15,
      itemStyle: { color: "#d96e51", opacity: 0.82, borderColor: "#ffffff", borderWidth: 0.8 },
      emphasis: { scale: 1.35, itemStyle: { opacity: 1 } },
    }],
  };
}

function barOptions(items, labels) {
  const values = items.slice(0, 12).reverse();
  return {
    animationDuration: 260,
    color: ["#d96e51"],
    grid: { left: 12, right: 18, top: 8, bottom: 26, containLabel: true },
    tooltip: { trigger: "axis", axisPointer: { type: "shadow" }, confine: true },
    xAxis: { type: "value", minInterval: 1, axisLabel: { color: "#6e7c80" }, splitLine: { lineStyle: { color: "#edf1ef" } } },
    yAxis: { type: "category", data: values.map((item) => item.value), axisLabel: { color: "#526063", width: 150, overflow: "truncate" }, axisTick: { show: false }, axisLine: { show: false } },
    series: [{ name: labels.pageviews || "Pageviews", type: "bar", data: values.map((item) => item.pageviews), barMaxWidth: 14 }],
  };
}

function pieOptions(items, labels) {
  return {
    animationDuration: 260,
    color: ["#2f655a", "#d96e51", "#d9a942", "#6e7c80", "#8aa9a2", "#c89583"],
    tooltip: { trigger: "item", confine: true, formatter: `{b}<br>${labels.pageviews || "Pageviews"}: {c} ({d}%)` },
    series: [{
      name: labels.share || "Share",
      type: "pie",
      radius: ["42%", "72%"],
      center: ["50%", "48%"],
      avoidLabelOverlap: true,
      label: { color: "#526063", fontSize: 10, formatter: "{b}" },
      labelLine: { length: 7, length2: 5 },
      data: items.slice(0, 8).map((item) => ({ name: item.value, value: item.pageviews })),
    }],
  };
}

function escapeHTML(value) {
  const element = document.createElement("span");
  element.textContent = String(value || "");
  return element.innerHTML;
}
