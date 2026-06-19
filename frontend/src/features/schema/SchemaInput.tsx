import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { Textarea } from "@/components/ui/textarea";
import type { ComponentSchema } from "@/types";
import { AudioInput } from "@/features/media/AudioInput";
import { FileInput } from "@/features/media/FileInput";
import { isLayoutComponent, numberProp, stringProp } from "./schema-utils";

export function SchemaInput({
  component,
  disabled,
  value,
  onChange,
}: {
  component: ComponentSchema;
  disabled: boolean;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const props = component.props ?? {};
  if (props.visible === false) {
    return null;
  }

  if (isLayoutComponent(component.type)) {
    return null;
  }

  const defaultValue = value ?? props.default ?? "";

  if (component.type === "number" || component.type === "slider") {
    const NumericInput = component.type === "slider" ? Slider : Input;
    return (
      <Field data-disabled={disabled || undefined}>
        <FieldLabel htmlFor={component.id}>{component.label}</FieldLabel>
        <NumericInput
          id={component.id}
          aria-label={component.label}
          disabled={disabled || props.disabled === true}
          type="number"
          min={numberProp(props.min)}
          max={numberProp(props.max)}
          step={numberProp(props.step)}
          value={String(defaultValue)}
          onChange={(event) => onChange(Number(event.target.value))}
        />
      </Field>
    );
  }

  if (component.type === "checkbox") {
    return (
      <Field data-disabled={disabled || undefined}>
        <label className="flex items-center gap-2 text-sm font-medium">
          <Checkbox
            aria-label={component.label}
            checked={Boolean(value ?? props.default ?? false)}
            disabled={disabled || props.disabled === true}
            onChange={(event) => onChange(event.target.checked)}
          />
          {component.label}
        </label>
      </Field>
    );
  }

  if (component.type === "dropdown") {
    return (
      <Field data-disabled={disabled || undefined}>
        <FieldLabel htmlFor={component.id}>{component.label}</FieldLabel>
        <Select
          id={component.id}
          aria-label={component.label}
          disabled={disabled || props.disabled === true}
          value={String(defaultValue)}
          onChange={(event) => onChange(event.target.value)}
        >
          {(component.choices ?? []).map((choice) => (
            <option key={choice} value={choice}>
              {choice}
            </option>
          ))}
        </Select>
      </Field>
    );
  }

  if (component.type === "audio") {
    return <AudioInput component={component} disabled={disabled} value={value} onChange={onChange} />;
  }

  if (component.type === "file" || component.type === "image") {
    return <FileInput component={component} disabled={disabled} value={value} onChange={onChange} />;
  }

  return (
    <Field data-disabled={disabled || undefined}>
      <FieldLabel htmlFor={component.id}>{component.label}</FieldLabel>
      <Textarea
        id={component.id}
        aria-label={component.label}
        disabled={disabled || props.disabled === true}
        placeholder={stringProp(props.placeholder)}
        rows={numberProp(props.rows) ?? 4}
        value={String(defaultValue)}
        onChange={(event) => onChange(event.target.value)}
      />
    </Field>
  );
}
