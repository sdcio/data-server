package validation

import (
	"context"
	"fmt"
	"strings"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/yang-parser/xpath"
	"github.com/sdcio/yang-parser/xpath/grammars/expr"
)

func validateMustStatements(ctx context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	// if no schema, then there is nothing to be done, return
	if e.GetSchema() == nil {
		return
	}

	var mustStatements []*sdcpb.MustStatement
	switch schem := e.GetSchema().GetSchema().(type) {
	case *sdcpb.SchemaElem_Container:
		mustStatements = schem.Container.GetMustStatements()
	case *sdcpb.SchemaElem_Leaflist:
		mustStatements = schem.Leaflist.GetMustStatements()
	case *sdcpb.SchemaElem_Field:
		mustStatements = schem.Field.GetMustStatements()
	}

	// a container schema with keys lives on the list's key-level node, one level above its
	// instances (see GetFirstAncestorWithSchema / populateSchema). must-statements declared on
	// a list must only ever be evaluated against a resolved instance (RFC 7950 §7.8.3) - at the
	// key-level node itself, sibling leaves of an instance don't exist as children, so any
	// must-statement referencing them would spuriously fail against an empty node-set.
	if containerSchema := e.GetSchema().GetContainer(); containerSchema != nil {
		if level := len(containerSchema.GetKeys()); level > 0 {
			descendKeyLevels(e, level, func(instance api.Entry) {
				evaluateMustStatements(ctx, instance, mustStatements, resultChan, stats)
			})
			return
		}
	}

	evaluateMustStatements(ctx, e, mustStatements, resultChan, stats)
}

// evaluateMustStatements evaluates mustStatements with e as the xpath context node. e must
// already be a resolved instance (or a non-list schema node, which is inherently an "instance" of
// itself) - never a list's key-level node.
func evaluateMustStatements(ctx context.Context, e api.Entry, mustStatements []*sdcpb.MustStatement, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	log := logf.FromContext(ctx)

	for _, must := range mustStatements {
		// extract actual must statement
		exprStr := must.Statement
		// init a ProgramBuilder
		prgbuilder := xpath.NewProgBuilder(exprStr)
		// init an ExpressionLexer
		lexer := expr.NewExprLex(exprStr, prgbuilder, nil)
		// parse the provided Must-Expression
		lexer.Parse()
		prog, err := lexer.CreateProgram(exprStr)
		if err != nil {
			owner := "unknown"
			highest := e.GetLeafVariants().GetHighestPrecedence(false, false, false)
			if highest != nil {
				owner = highest.Owner()
			}
			resultChan <- types.NewValidationResultEntry(owner, err, types.ValidationResultEntryTypeError)
			return
		}
		machine := xpath.NewMachine(exprStr, prog, exprStr)

		// run the must statement evaluation virtual machine
		yctx := xpath.NewCtxFromCurrent(ctx, machine, newYangParserEntryAdapter(ctx, e))
		yctx.SetDebug(false)

		res1 := yctx.Run()
		// retrieve the boolean result of the execution
		result, err := res1.GetBoolResult()
		if !result || err != nil {
			if err == nil {
				err = fmt.Errorf("error path: %s, must-statement [%s] %s", e.SdcpbPath().ToXPath(false), must.Statement, must.Error)
			} else {
				err = fmt.Errorf("error path: %s, must-statement [%s]: %w", e.SdcpbPath().ToXPath(false), must.Statement, err)
			}
			if strings.Contains(err.Error(), "Stack underflow") {
				log.Error(err, "stack underflow", "path", e.SdcpbPath().ToXPath(false), "must-expression", exprStr)
				continue
			}
			owner := "unknown"

			// must statement might be assigned on a container, hence we might not have any LeafVariants
			leafVariants := e.GetLeafVariants()
			if leafVariants.Length() > 0 {
				highest := leafVariants.GetHighestPrecedence(false, false, false)
				if highest != nil {
					owner = highest.Owner()
				}
			}
			resultChan <- types.NewValidationResultEntry(owner, err, types.ValidationResultEntryTypeError)
		}
	}
	stats.Add(types.StatTypeMustStatement, uint32(len(mustStatements)))
}
