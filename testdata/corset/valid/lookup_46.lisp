;; As lookup_17, but without selectors.
(defcolumns (X :i16) (Y :i16))
;; fragmented lookup with single fragment.
(deflookup test ((_ X)) ((_ Y)))
