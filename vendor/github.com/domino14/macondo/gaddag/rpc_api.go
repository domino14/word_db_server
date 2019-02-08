// This is the RPC API for the gaddag package.
package gaddag

import "net/http"

type GaddagServiceArgs struct {
	Filename string `json:"filename"`
	Minimize bool   `json:"minimize"`
}

type GaddagServiceReply struct {
	Message string `json:"message"`
}

type GaddagService struct{}

func (g *GaddagService) Generate(r *http.Request, args *GaddagServiceArgs,
	reply *GaddagServiceReply) error {
	GenerateGaddag(args.Filename, args.Minimize, true)
	reply.Message = "Done"
	return nil
}

func (g *GaddagService) GenerateDawg(r *http.Request, args *GaddagServiceArgs,
	reply *GaddagServiceReply) error {
	GenerateDawg(args.Filename, args.Minimize, true)
	reply.Message = "Done"
	return nil
}

// func (g *GaddagService) LoadDawg(r *http.Request, args *GaddagServiceArgs,
// 	reply *GaddagServiceReply) error {
// 	RpcDawg = SimpleDawg(LoadGaddag(args.Filename))
// 	reply.Message = "Done"
// 	return nil
// }