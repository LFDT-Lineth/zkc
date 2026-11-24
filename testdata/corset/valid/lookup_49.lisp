;; As lookup_20, but with only one target having a selector.
;; fragmented lookup with two targets
(deflookup test ((_ s.X)) ((t1.one t1.Y) (_ t2.Z)))
;;
(module t1)
(defcolumns (Y :i16) (one :i1 :constant 1))

(module t2)
(defcolumns (Z :i16))

(module s)
(defcolumns (X :i16))
