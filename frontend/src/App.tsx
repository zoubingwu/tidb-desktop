import MainDataView from "@/components/MainDataView";
import TitleBar from "@/components/TitleBar";
import WelcomeScreen from "@/components/WelcomeScreen";
import { useMemoizedFn } from "ahooks";
import { useEffect, useState } from "react";
import { Disconnect } from "wailsjs/go/main/App";
import { services } from "wailsjs/go/models";
import { EventsOn } from "wailsjs/runtime";

type ViewState = "welcome" | "main";

function App() {
  const [currentView, setCurrentView] = useState<ViewState>("welcome");
  const [connectionDetails, setConnectionDetails] =
    useState<services.ConnectionDetails | null>(null);

  const navigateToMain = useMemoizedFn(
    (details: services.ConnectionDetails) => {
      setConnectionDetails(details);
      setCurrentView("main");
    },
  );

  useEffect(() => {
    const cleanupEstablished = EventsOn(
      "connection:established",
      (details: services.ConnectionDetails) => {
        navigateToMain(details);
      },
    );
    const cleanupDisconnected = EventsOn("connection:disconnected", () => {
      handleDisconnect();
    });

    return () => {
      cleanupEstablished();
      cleanupDisconnected();
    };
  }, []);

  const handleDisconnect = useMemoizedFn(() => {
    setConnectionDetails(null);
    setCurrentView("welcome");
  });

  const triggerDisconnect = useMemoizedFn(() => {
    Disconnect();
  });

  const renderView = () => {
    switch (currentView) {
      case "welcome":
        return <WelcomeScreen />;
      case "main":
        return (
          <MainDataView
            onClose={triggerDisconnect}
            connectionDetails={connectionDetails}
          />
        );
      default:
        return <div>Unknown View</div>;
    }
  };

  const connectionName = connectionDetails
    ? connectionDetails?.name ||
      `${connectionDetails?.user}@${connectionDetails?.host}:${connectionDetails?.port}`
    : "";

  const title = connectionName
    ? `TiDB Desktop - ${connectionName}`
    : "TiDB Desktop";

  return (
    <div id="App" className="h-screen w-screen flex flex-col">
      <TitleBar title={title} />
      <div className="flex-grow overflow-auto">{renderView()}</div>
    </div>
  );
}

export default App;
