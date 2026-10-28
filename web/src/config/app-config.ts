import packageJson from "../../package.json";

const currentYear = new Date().getFullYear();

export const APP_CONFIG = {
  name: "ARTEX",
  version: packageJson.version,
  copyright: `© ${currentYear}, ARTEX.`,
  meta: {
    title: "ARTEX — 个人工作台",
    description: "个人待办、项目、日历、笔记与自动化执行工作台",
  },
};
