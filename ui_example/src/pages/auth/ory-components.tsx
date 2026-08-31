import type {
  OryFlowComponentOverrides,
  OryNodeButtonProps,
  OryNodeInputProps,
} from "@ory/elements-react";
import { getNodeLabel } from "@ory/client-fetch";
import {
  uiTextToFormattedMessage,
  useOryConfiguration,
} from "@ory/elements-react";
import {
  forwardRef,
  useState,
  type PropsWithChildren,
  type Ref,
} from "react";
import { useIntl } from "react-intl";

// 自定义 Card.Root 后必须保留 ory-elements，Ory 的默认子组件依赖这个作用域。
function AuthCardRoot({ children }: PropsWithChildren) {
  return <div className="ory-elements auth-element-card">{children}</div>;
}

// Logo 是品牌结构定制点；名称从 Ory 配置读取，避免把适配器写死成某个产品。
function AuthCardLogo() {
  const { project } = useOryConfiguration();

  return (
    <div className="auth-element-logo">
      <span className="brand-mark__icon">D</span>
      <span>
        <strong>{project.name}</strong>
        <small>Identity Lab</small>
      </span>
    </div>
  );
}

function assignRef<T>(ref: Ref<T> | undefined, value: T | null) {
  if (typeof ref === "function") {
    ref(value);
  } else if (ref) {
    (ref as { current: T | null }).current = value;
  }
}

// Input 是完整替换，因此同时读取 attributes（Kratos 约束）和 inputProps（表单运行时字段）。
const AuthInput = forwardRef<HTMLInputElement, OryNodeInputProps>(
  ({ attributes, inputProps }, forwardedRef) => {
    const [showPassword, setShowPassword] = useState(false);
    const isPassword = inputProps.type === "password";
    const { ref: oryRef, type, ...restInputProps } = inputProps;
    const inputType = isPassword && showPassword ? "text" : type;
    const setInputRef = (element: HTMLInputElement | null) => {
      assignRef(oryRef, element);
      assignRef(forwardedRef, element);
    };

    if (type === "hidden") {
      return (
        <input
          {...restInputProps}
          name={attributes.name}
          required={attributes.required}
          disabled={attributes.disabled || inputProps.disabled}
          autoComplete={inputProps.autoComplete ?? attributes.autocomplete}
          maxLength={inputProps.maxLength ?? attributes.maxlength}
          ref={setInputRef}
          type={type}
          data-testid={`ory/form/node/input/${inputProps.name}`}
        />
      );
    }

    return (
      <div
        className={`auth-element-input${isPassword ? " auth-element-password" : ""}`}
      >
        <input
          {...restInputProps}
          name={attributes.name}
          required={attributes.required}
          disabled={attributes.disabled || inputProps.disabled}
          autoComplete={inputProps.autoComplete ?? attributes.autocomplete}
          maxLength={inputProps.maxLength ?? attributes.maxlength}
          ref={setInputRef}
          type={inputType}
          data-testid={`ory/form/node/input/${inputProps.name}`}
        />
        {isPassword && (
          <button
            className="auth-element-password-toggle"
            type="button"
            onClick={() => setShowPassword((visible) => !visible)}
            aria-label={showPassword ? "Hide password" : "Show password"}
          >
            {showPassword ? "Hide" : "Show"}
          </button>
        )}
      </div>
    );
  },
);
AuthInput.displayName = "AuthInput";

function AuthButton({ node, buttonProps, isSubmitting }: OryNodeButtonProps) {
  const label = getNodeLabel(node);
  const intl = useIntl();

  return (
    <button
      {...buttonProps}
      className="auth-element-button"
      disabled={isSubmitting || buttonProps.disabled}
      data-loading={isSubmitting}
      data-testid={`ory/form/node/button/${node.attributes.name}`}
      aria-busy={isSubmitting}
    >
      {isSubmitting
        ? "Submitting…"
        : label
          ? uiTextToFormattedMessage(label, intl)
          : node.attributes.name}
    </button>
  );
}

// 只覆盖结构确实需要变化的组件；Label、CodeInput、SsoButton 等继续使用默认实现。
export const authElementComponents: OryFlowComponentOverrides = {
  Card: {
    Root: AuthCardRoot,
    Logo: AuthCardLogo,
  },
  Node: {
    Input: AuthInput,
    Button: AuthButton,
  },
};
