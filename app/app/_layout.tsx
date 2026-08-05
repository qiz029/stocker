import React, { useEffect } from "react";
import { ActivityIndicator, View } from "react-native";
import { Stack, useRouter, useSegments } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { SessionProvider, useSession } from "../src/session";
import { colors } from "../src/theme";

/** Session gate: no user → /login; logged in on /login → rooms list. */
function Gate({ children }: { children: React.ReactNode }) {
  const { user, loading } = useSession();
  const segments = useSegments();
  const router = useRouter();
  useEffect(() => {
    if (loading) return;
    const onLogin = segments[0] === "login";
    const onProfile = segments[0] === "profile";
    const profileDone = user?.profile_complete !== false;
    if (!user && !onLogin) router.replace("/login");
    else if (user && !profileDone && !onProfile) router.replace("/profile");
    else if (user && profileDone && (onLogin || onProfile)) router.replace("/");
  }, [user, loading, segments, router]);
  if (loading) {
    return (
      <View style={{ flex: 1, backgroundColor: colors.bg, alignItems: "center", justifyContent: "center" }}>
        <ActivityIndicator color={colors.up} />
      </View>
    );
  }
  return <>{children}</>;
}

export default function RootLayout() {
  return (
    <SafeAreaProvider>
      <SessionProvider>
        <StatusBar style="light" />
        <Gate>
          <Stack screenOptions={{
            headerShown: false,
            contentStyle: { backgroundColor: colors.bg },
          }} />
        </Gate>
      </SessionProvider>
    </SafeAreaProvider>
  );
}
