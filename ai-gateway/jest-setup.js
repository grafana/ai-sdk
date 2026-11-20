// Jest setup provided by Grafana scaffolding
import './.config/jest-setup';

// jsdom has no ResizeObserver, which @grafana/ui's Drawer uses.
global.ResizeObserver = class ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
};
