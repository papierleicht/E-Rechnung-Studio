export type ShopCredentialKey =
  | "accessToken"
  | "clientId"
  | "clientSecret"
  | "consumerKey"
  | "consumerSecret"
  | "apiKey"
  | "username"
  | "password";

export interface ShopCredentialField {
  key: ShopCredentialKey;
  label: string;
}

export interface ShopProvider {
  id: string;
  label: string;
  guide: string;
  setup: string;
  fields: readonly ShopCredentialField[];
  readResources?: readonly string[];
}

export const wixSiteIdPattern =
  "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";

export const shopProviders: readonly ShopProvider[] = [
  {
    id: "shopify",
    label: "Shopify",
    guide:
      "https://shopify.dev/docs/apps/build/authentication-authorization/client-credentials-grant",
    setup:
      "Verwende eine Shopify-App mit Lesezugriff auf Bestellungen (read_orders). Für ältere Bestellungen benötigt die App zusätzlich Zugriff auf alle Bestellungen (read_all_orders).",
    fields: [{ key: "accessToken", label: "Admin-API-Zugriffstoken" }],
  },
  {
    id: "woocommerce",
    label: "WooCommerce",
    guide: "https://developer.woocommerce.com/docs/apis/rest-api/",
    setup:
      "Erstelle in WooCommerce unter Einstellungen → Erweitert → REST-API einen Schlüssel mit der Berechtigung „Lesen“.",
    fields: [
      { key: "consumerKey", label: "Consumer Key" },
      { key: "consumerSecret", label: "Consumer Secret" },
    ],
  },
  {
    id: "shopware",
    label: "Shopware 6",
    guide: "https://docs.shopware.com/de/shopware-6-de/einstellungen/system/integrationen",
    setup:
      "Leg in Shopware 6 unter Einstellungen → System → Integrationen einen Zugang an. Die zugewiesene Rolle benötigt Leserechte für Bestellungen und verknüpfte Kundendaten.",
    fields: [
      { key: "clientId", label: "Integrations-Zugangs-ID" },
      { key: "clientSecret", label: "Sicherheitsschlüssel" },
    ],
  },
  {
    id: "gambio",
    label: "Gambio",
    guide: "https://developers.gambio.de/docs/API/",
    setup:
      "Verwende den Benutzernamen bzw. die E-Mail-Adresse und das Passwort eines Gambio-Administrators oder API-Benutzers mit Bestellzugriff über die REST-API v2.",
    fields: [
      { key: "username", label: "Benutzername / E-Mail" },
      { key: "password", label: "Passwort" },
    ],
  },
  {
    id: "wix",
    label: "Wix",
    guide:
      "https://dev.wix.com/docs/develop-websites-sdk/code-your-site/authorization/make-rest-api-calls-with-an-api-key",
    setup:
      "Erstelle in deinem Wix-Konto einen API-Schlüssel mit der Berechtigung „eCommerce Read Orders“. Der Schlüssel muss zum Konto gehören, dem die Website gehört, und Zugriff auf diese Website haben.",
    fields: [{ key: "apiKey", label: "Wix-API-Schlüssel" }],
  },
  {
    id: "prestashop",
    label: "PrestaShop",
    guide: "https://devdocs.prestashop-project.org/9/webservice/tutorials/creating-access/",
    setup:
      "Aktiviere in PrestaShop unter Erweiterte Einstellungen → Webservice den Webservice und erstelle einen Zugangsschlüssel mit den unten genannten GET-Leserechten.",
    fields: [{ key: "apiKey", label: "Webservice-Schlüssel" }],
    readResources: [
      "orders",
      "order_details",
      "addresses",
      "customers",
      "currencies",
      "countries",
      "taxes",
      "order_states",
      "order_payments",
      "order_slip",
    ],
  },
];

export const shopifyClientFields: readonly ShopCredentialField[] = [
  { key: "clientId", label: "Client-ID" },
  { key: "clientSecret", label: "Client-Secret" },
];

export function shopProviderByID(id: string): ShopProvider | undefined {
  return shopProviders.find((provider) => provider.id === id);
}

/** A new object also prevents secrets from following a provider or authentication change. */
export function emptyShopCredentials(): Record<ShopCredentialKey, string> {
  return {
    accessToken: "",
    clientId: "",
    clientSecret: "",
    consumerKey: "",
    consumerSecret: "",
    apiKey: "",
    username: "",
    password: "",
  };
}
