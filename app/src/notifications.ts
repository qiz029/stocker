import AsyncStorage from "@react-native-async-storage/async-storage";
import * as Device from "expo-device";
import * as Notifications from "expo-notifications";
import { api } from "@core/api";
import type { Lang } from "@core/i18n";

const TOKEN_KEY = "stocker.pushToken";

// Foreground notifications: show them like a normal banner.
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldPlaySound: false,
    shouldSetBadge: false,
    shouldShowBanner: true,
    shouldShowList: true,
  }),
});

/**
 * Ask for notification permission, fetch the Expo push token, and register
 * it with the backend. Fully best-effort: any failure (simulator, denied
 * permission, no EAS project yet, network) degrades silently and never
 * blocks login.
 */
export async function registerPushToken(lang: Lang): Promise<void> {
  try {
    if (!Device.isDevice) return; // simulator: no push tokens
    const perms = await Notifications.getPermissionsAsync();
    let status = perms.status;
    if (status !== "granted") {
      status = (await Notifications.requestPermissionsAsync()).status;
    }
    if (status !== "granted") return;
    const { data: token } = await Notifications.getExpoPushTokenAsync();
    await AsyncStorage.setItem(TOKEN_KEY, token);
    await api.post("/api/me/push-token", { token, lang });
  } catch {
    /* push is best-effort */
  }
}

/** Unregister the device token on logout; best-effort like registration. */
export async function unregisterPushToken(): Promise<void> {
  try {
    const token = await AsyncStorage.getItem(TOKEN_KEY);
    if (token) {
      await api.del(`/api/me/push-token?token=${encodeURIComponent(token)}`);
      await AsyncStorage.removeItem(TOKEN_KEY);
    }
  } catch {
    /* push is best-effort */
  }
}
