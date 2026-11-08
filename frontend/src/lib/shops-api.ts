import * as Native from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/shopservice";
import type {
  Connection,
  Credentials,
  Order,
  Overview,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/shops/models";

export type ShopConnection = Connection;
export type ShopCredentials = Credentials;
export type ShopOrder = Order;
export type ShopOverview = Omit<Overview, "connections" | "orders"> & {
  connections: Connection[];
  orders: Order[];
};

export const shops = {
  overview: async (): Promise<ShopOverview> => {
    const result = await Native.Overview();
    return { ...result, connections: result.connections ?? [], orders: result.orders ?? [] };
  },
  connect: (connection: Connection, credentials: Credentials) =>
    Native.Connect(connection, credentials),
  sync: (id: string) => Native.Sync(id),
  setEnabled: (id: string, enabled: boolean) => Native.SetEnabled(id, enabled),
  disconnect: (id: string) => Native.Disconnect(id),
  createInvoice: (orderID: string) => Native.CreateInvoice(orderID),
};
