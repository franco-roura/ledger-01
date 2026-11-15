"use client";

import { useMutation, useQuery } from "@connectrpc/connect-query";
import {
  Button,
  Field,
  fault,
  Input,
  Panel,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
} from "@franco_roura/module-01";
import { CategoryIcon } from "@franco_roura/module-01/finance";
import { CategoryKind, ImportService } from "@ledger-01/api";
import { Plus, Trash2 } from "lucide-react";
import { useId, useState } from "react";
import { toast } from "sonner";
import { CategoryPicker } from "@/components/category-picker";
import { useCategories } from "@/lib/api/hooks";

/** "Note contains … → category" rules that fill in categories when importing bank files. */
export default function ImportRulesPage() {
  const id = useId();
  const { categories, byId } = useCategories();
  const { data, isPending } = useQuery(ImportService.method.listCategoryRules, {});
  const rules = data?.rules ?? [];
  const create = useMutation(ImportService.method.createCategoryRule);
  const remove = useMutation(ImportService.method.deleteCategoryRule);
  const [pattern, setPattern] = useState("");
  const [kind, setKind] = useState<CategoryKind>(CategoryKind.EXPENSE);
  const [categoryId, setCategoryId] = useState("");
  const [errors, setErrors] = useState<{ pattern?: string; cat?: string }>({});

  async function add(e: React.FormEvent) {
    e.preventDefault();
    if (!pattern.trim()) {
      setErrors({ pattern: "Enter the text to look for" });
      return fault(`${id}-pattern`);
    }
    if (!categoryId) {
      setErrors({ cat: "Choose a category" });
      return fault(`${id}-cat`);
    }
    try {
      await create.mutateAsync({ pattern, categoryId });
    } catch {
      return; // toasted
    }
    toast.success("Rule saved");
    setPattern("");
    setCategoryId("");
  }

  return (
    <div className="flex flex-col gap-6">
      <Panel serial="IMP-01 · NEW">
        <form onSubmit={add} className="flex flex-col gap-4">
          <p className="text-body-sm text-ink-soft">
            When an imported row's note contains the text, it gets the category. The longest
            matching text wins.
          </p>
          <div className="grid gap-4 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,2fr)]">
            <Field label="When the note contains" htmlFor={`${id}-pattern`} error={errors.pattern}>
              <Input
                id={`${id}-pattern`}
                value={pattern}
                maxLength={100}
                placeholder="e.g. Corner Grocer"
                onChange={(e) => {
                  setPattern(e.target.value);
                  setErrors({});
                }}
              />
            </Field>
            <Field label="Kind" htmlFor={`${id}-kind`}>
              <Select
                value={String(kind)}
                onValueChange={(v) => {
                  setKind(Number(v) as CategoryKind);
                  setCategoryId("");
                }}
              >
                <SelectTrigger id={`${id}-kind`} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={String(CategoryKind.EXPENSE)}>Expenses</SelectItem>
                  <SelectItem value={String(CategoryKind.INCOME)}>Income</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label="Use the category" htmlFor={`${id}-cat`} error={errors.cat}>
              <CategoryPicker
                id={`${id}-cat`}
                categories={categories.filter((c) => c.kind === kind)}
                walletId=""
                value={categoryId}
                onChange={(v) => {
                  setCategoryId(v);
                  setErrors({});
                }}
              />
            </Field>
          </div>
          <div>
            <Button type="submit" disabled={create.isPending}>
              <Plus strokeWidth={1.25} /> Add rule
            </Button>
          </div>
        </form>
      </Panel>

      <Panel serial="IMP-02 · ALL">
        {isPending ? (
          <div className="flex flex-col gap-3">
            {[0, 1].map((i) => (
              <Skeleton key={i} className="h-10" />
            ))}
          </div>
        ) : rules.length === 0 ? (
          <p className="text-body-sm text-ink-soft">
            No rules yet. Add them here, or from a row while reviewing an import.
          </p>
        ) : (
          <ul className="flex flex-col">
            {rules.map((r) => {
              const c = byId.get(r.categoryId);
              return (
                <li
                  key={r.id}
                  className="flex items-center gap-3 border-b border-seam/60 py-3 last:border-b-0"
                >
                  <span className="min-w-0 flex-1 truncate text-body-sm text-ink">
                    Contains “{r.pattern}”
                  </span>
                  <span className="flex min-w-0 items-center gap-2 text-label text-ink">
                    {c && <CategoryIcon icon={c.icon} color={c.color} size="sm" />}
                    <span className="truncate">{c?.name ?? "Unknown category"}</span>
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={`Delete the rule for “${r.pattern}”`}
                    onClick={() => remove.mutate({ id: r.id })}
                    disabled={remove.isPending}
                  >
                    <Trash2 strokeWidth={1.25} />
                  </Button>
                </li>
              );
            })}
          </ul>
        )}
      </Panel>
    </div>
  );
}
