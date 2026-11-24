;; fragmented lookup with two targets
(deflookup test ((s.one s.X)) ((t1.one t1.Y) (t2.one t2.Z)))
;;
(module t1)
(defcolumns (Y :i16) (one :i1 :constant 1))

(module t2)
(defcolumns (Z :i16) (one :i1 :constant 1))

(module s)
(defcolumns (X :i16) (one :i1 :constant 1))
