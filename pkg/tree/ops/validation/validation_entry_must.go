package validation

import (
	"context"
	"fmt"
	"strings"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	logf "github.com/sdcio/logger"
	"github.com/sdcio/yang-parser/xpath"
	"github.com/sdcio/yang-parser/xpath/grammars/expr"
)

func validateMustStatements(ctx context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	validateMustStatementsWithTracer(ctx, e, resultChan, stats, nil)
}

func validateMustStatementsWithTracer(ctx context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats, mt MustTracer) {
	log := logf.FromContext(ctx)

	if e.GetSchema() == nil {
		return
	}

	mustStatements, schemaKind := extractEntryMusts(e)
	if len(mustStatements) == 0 {
		return
	}

	for i, must := range mustStatements {
		exprStr := must.Statement
		prgbuilder := xpath.NewProgBuilder(exprStr)
		lexer := expr.NewExprLex(exprStr, prgbuilder, nil)
		lexer.Parse()
		prog, err := lexer.CreateProgram(exprStr)
		if err != nil {
			if mt != nil {
				if et := mt.BeginMustTrace(buildMustTraceMeta(e, schemaKind, must)); et != nil {
					et.End(MustTraceOutcome{
						Status: MustTraceStatusCompileError,
						Error:  err.Error(),
					})
				}
				for _, skipped := range mustStatements[i+1:] {
					if et := mt.BeginMustTrace(buildMustTraceMeta(e, schemaKind, skipped)); et != nil {
						et.End(MustTraceOutcome{
							Status:     MustTraceStatusSkipped,
							SkipReason: "preceding must statement failed to compile",
						})
					}
				}
			}
			resultChan <- types.NewValidationResultEntry(entryOwner(e), err, types.ValidationResultEntryTypeError)
			return
		}
		machine := xpath.NewMachine(exprStr, prog, exprStr)

		var evalTracer MustEvalTracer
		if mt != nil {
			evalTracer = mt.BeginMustTrace(buildMustTraceMeta(e, schemaKind, must))
			if evalTracer != nil {
				evalTracer.TraceEvent(xpath.TraceEvent{
					Kind: TraceEventMustProgram,
					Expr: exprStr,
					Details: map[string]any{
						"program": machine.PrintMachine(),
					},
				})
			}
		}

		yctx := xpath.NewCtxFromCurrent(ctx, machine, newYangParserEntryAdapterWithTracer(ctx, e, evalTracer))
		yctx.SetDebug(false)
		yctx.SetTracer(evalTracer)

		res1 := yctx.Run()
		result, err := res1.GetBoolResult()
		if !result || err != nil {
			rawErr := err
			if err == nil {
				err = fmt.Errorf("error path: %s, must-statement [%s] %s", e.SdcpbPath().ToXPath(false), must.Statement, must.Error)
			}
			if strings.Contains(err.Error(), "Stack underflow") {
				if evalTracer != nil {
					evalTracer.End(MustTraceOutcome{
						Status: MustTraceStatusStackUnderflow,
						Error:  err.Error(),
					})
				}
				log.Error(err, "stack underflow", "path", e.SdcpbPath().ToXPath(false), "must-expression", exprStr)
				continue
			}
			if evalTracer != nil {
				if rawErr != nil {
					evalTracer.End(MustTraceOutcome{
						Status: MustTraceStatusRuntimeError,
						Error:  err.Error(),
					})
				} else {
					b := false
					evalTracer.End(MustTraceOutcome{
						Status: MustTraceStatusFailed,
						Result: &b,
						Error:  err.Error(),
					})
				}
			}

			resultChan <- types.NewValidationResultEntry(entryOwner(e), err, types.ValidationResultEntryTypeError)
			continue
		}

		if evalTracer != nil {
			b := true
			evalTracer.End(MustTraceOutcome{
				Status: MustTraceStatusPassed,
				Result: &b,
			})
		}
	}
	stats.Add(types.StatTypeMustStatement, uint32(len(mustStatements)))
}
