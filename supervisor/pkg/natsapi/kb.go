package natsapi

import "github.com/quarkloop/pkg/serviceapi/clientcontract"

func (s *Server) getKB(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.KBRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	store, err := s.store.KB(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	value, err := store.Get(payload.Namespace, payload.Key)
	if err != nil {
		return nil, err
	}
	return clientcontract.KBValueResponse{Value: append([]byte(nil), value...)}, nil
}

func (s *Server) setKB(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.KBSetRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	store, err := s.store.KB(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	if err := store.Set(payload.Namespace, payload.Key, append([]byte(nil), payload.Value...)); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func (s *Server) deleteKB(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.KBRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	store, err := s.store.KB(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	if err := store.Delete(payload.Namespace, payload.Key); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func (s *Server) listKB(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.KBListRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	store, err := s.store.KB(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	keys, err := store.List(payload.Namespace)
	if err != nil {
		return nil, err
	}
	return clientcontract.KBListResponse{Keys: append([]string(nil), keys...)}, nil
}
