import { Component, type ComponentType, type ReactNode } from "react";

import { type Customization, type SlotName, type SlotProps, activeCustomization } from "@/lib/customization";
import { useUserInfo } from "@/lib/use-user-info";

// CustomSlot renders the part of the site that a customization script may
// replace, or the built-in component when it does not. A customization that
// fails to render falls back to the built-in component, so a broken script
// cannot take the page down.
export function CustomSlot({
  name,
  fallback: Default,
  customization = activeCustomization(),
  onError,
}: {
  name: SlotName;
  fallback: ComponentType;
  // customization is the one to render, the published one by default.
  customization?: Customization | null;
  onError?: (error: unknown) => void;
}) {
  const user = useUserInfo();
  const Custom = customization?.slots[name];
  if (!Custom) return <Default />;
  return (
    <SlotBoundary name={name} component={Custom} fallback={<Default />} onError={onError}>
      <Custom Default={Default} user={user} />
    </SlotBoundary>
  );
}

interface SlotBoundaryProps {
  name: SlotName;
  // component is the one rendered, and another one renders again.
  component: ComponentType<SlotProps>;
  fallback: ReactNode;
  onError?: (error: unknown) => void;
  children: ReactNode;
}

interface SlotBoundaryState {
  failed: boolean;
  component: ComponentType<SlotProps>;
}

class SlotBoundary extends Component<SlotBoundaryProps, SlotBoundaryState> {
  constructor(props: SlotBoundaryProps) {
    super(props);
    this.state = { failed: false, component: props.component };
  }

  static getDerivedStateFromProps(props: SlotBoundaryProps, state: SlotBoundaryState) {
    return props.component === state.component ? null : { failed: false, component: props.component };
  }

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: unknown) {
    console.error(`Failed to render the customized ${this.props.name}`, error);
    this.props.onError?.(error);
  }

  render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}
